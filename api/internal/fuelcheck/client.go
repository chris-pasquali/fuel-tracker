package fuelcheck

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"fillwise/internal/config"
	"fillwise/internal/domain"
)

type Client struct {
	cfg    config.Config
	http   *http.Client
	token  string
	expiry time.Time
}

type rawStation struct {
	Brand     string `json:"brand"`
	Code      any    `json:"code"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	StationID string `json:"stationid"`
	IsAdBlue  bool   `json:"isAdBlueAvailable"`
	Location  struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"location"`
}

type rawPrice struct {
	StationCode any     `json:"stationcode"`
	FuelType    string  `json:"fueltype"`
	Price       float64 `json:"price"`
	LastUpdated string  `json:"lastupdated"`
}

type Payload struct {
	Stations []rawStation `json:"stations"`
	Prices   []rawPrice   `json:"prices"`
}

func New(cfg config.Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) tokenHeader() (string, error) {
	if c.token != "" && time.Now().Before(c.expiry) {
		return c.token, nil
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.onegov.nsw.gov.au/oauth/client_credential/accesstoken?grant_type=client_credentials", nil)
	if err != nil {
		return "", err
	}
	basic := base64.StdEncoding.EncodeToString([]byte(c.cfg.NSWFuelAPIKey + ":" + c.cfg.NSWFuelAPISecret))
	req.Header.Set("Authorization", "Basic "+basic)
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("fuelcheck auth %s", res.Status)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("fuelcheck auth: empty token")
	}
	c.token = body.AccessToken
	c.expiry = time.Now().Add(10 * time.Hour)
	return c.token, nil
}

func (c *Client) FetchAll() (*Payload, error) {
	token, err := c.tokenHeader()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.onegov.nsw.gov.au/FuelPriceCheck/v1/fuel/prices", nil)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(time.FixedZone("AEST", 10*3600))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("apikey", c.cfg.NSWFuelAPIKey)
	req.Header.Set("transactionid", fmt.Sprintf("fillwise_%d", time.Now().UnixMilli()))
	req.Header.Set("requesttimestamp", now.Format("02/01/2006 03:04:05 PM"))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 400))
		return nil, fmt.Errorf("fuelcheck prices %s: %s", res.Status, b)
	}
	var payload Payload
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func LoadSnapshot(path string) (*Payload, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload Payload
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func WriteSnapshot(path string, payload *Payload) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func dirOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}

func FilterSydney(payload *Payload) ([]domain.Station, []domain.Observation) {
	stations := make([]domain.Station, 0, len(payload.Stations))
	codes := map[string]domain.Station{}
	for _, raw := range payload.Stations {
		code := stringify(raw.Code)
		if code == "" || !domain.IsGreaterSydney(raw.Address, raw.Location.Latitude, raw.Location.Longitude) {
			continue
		}
		suburb, pc := domain.ParseAddress(raw.Address)
		if suburb == "" {
			suburb = "Unknown"
		}
		brand := strings.TrimSpace(strings.ReplaceAll(raw.Brand, " logo", ""))
		if brand == "" {
			brand = "Independent"
		}
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			name = brand + " " + code
		}
		st := domain.Station{
			Code: code, StationID: raw.StationID, Brand: brand, Name: name,
			Address: raw.Address, Suburb: suburb, Postcode: pc, Region: domain.RegionFor(pc),
			Lat: raw.Location.Latitude, Lon: raw.Location.Longitude, IsAdBlue: raw.IsAdBlue,
		}
		stations = append(stations, st)
		codes[code] = st
	}
	obs := make([]domain.Observation, 0, len(payload.Prices))
	for _, p := range payload.Prices {
		code := stringify(p.StationCode)
		fuel := strings.ToUpper(p.FuelType)
		if _, ok := codes[code]; !ok {
			continue
		}
		if _, ok := domain.Tracked[fuel]; !ok || p.Price <= 0 {
			continue
		}
		obs = append(obs, domain.Observation{StationCode: code, FuelType: fuel, PriceCPL: p.Price, LastUpdated: p.LastUpdated})
	}
	return stations, obs
}

func SydneyOnly(payload *Payload) *Payload {
	out := &Payload{}
	keep := map[string]struct{}{}
	for _, s := range payload.Stations {
		if domain.IsGreaterSydney(s.Address, s.Location.Latitude, s.Location.Longitude) {
			out.Stations = append(out.Stations, s)
			keep[stringify(s.Code)] = struct{}{}
		}
	}
	for _, p := range payload.Prices {
		if _, ok := keep[stringify(p.StationCode)]; ok {
			out.Prices = append(out.Prices, p)
		}
	}
	return out
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
