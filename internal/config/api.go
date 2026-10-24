package config

type API struct {
	Enabled bool   `default:"false" envconfig:"ENABLED"` // internal use only
	Addr    string `default:":8080" envconfig:"ADDR"`
}
