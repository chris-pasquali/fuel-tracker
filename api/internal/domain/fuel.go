package domain

type Fuel struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Family          string `json:"family"`
	Unit            string `json:"unit"`
	TypicalCycleCPL float64 `json:"-"`
	Sort            int    `json:"-"`
}

var Fuels = []Fuel{
	{Code: "E10", Name: "E10 (ethanol 91)", Family: "petrol", Unit: "¢/L", TypicalCycleCPL: 18, Sort: 1},
	{Code: "U91", Name: "Unleaded 91", Family: "petrol", Unit: "¢/L", TypicalCycleCPL: 18, Sort: 2},
	{Code: "P95", Name: "Premium 95", Family: "petrol", Unit: "¢/L", TypicalCycleCPL: 16, Sort: 3},
	{Code: "P98", Name: "Premium 98", Family: "petrol", Unit: "¢/L", TypicalCycleCPL: 16, Sort: 4},
	{Code: "DL", Name: "Diesel", Family: "diesel", Unit: "¢/L", TypicalCycleCPL: 8, Sort: 5},
	{Code: "PDL", Name: "Premium diesel", Family: "diesel", Unit: "¢/L", TypicalCycleCPL: 8, Sort: 6},
	{Code: "LPG", Name: "LPG", Family: "gas", Unit: "¢/L", TypicalCycleCPL: 6, Sort: 7},
	{Code: "E85", Name: "E85", Family: "petrol", Unit: "¢/L", TypicalCycleCPL: 14, Sort: 8},
	{Code: "B20", Name: "Biodiesel B20", Family: "diesel", Unit: "¢/L", TypicalCycleCPL: 6, Sort: 9},
}

var Tracked = map[string]Fuel{}

func init() {
	for _, f := range Fuels {
		Tracked[f.Code] = f
	}
}

func OrDefault(code string) Fuel {
	if f, ok := Tracked[code]; ok {
		return f
	}
	return Tracked["E10"]
}

func Catalogue() []Fuel {
	return Fuels
}
