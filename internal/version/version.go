// Package version holds product/API compatibility metadata shared by all Wayshard binaries.
package version

const Product = "Wayshard"

// Compatibility is the control-plane API major that clients advertise and the server accepts.
const Compatibility = 1

var (
	Version = "0.0.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

type Info struct {
	Product       string `json:"product"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Date          string `json:"date"`
	API           int    `json:"api"`
	Compatibility int    `json:"compatibility"`
}

func Current() Info {
	return Info{
		Product:       Product,
		Version:       Version,
		Commit:        Commit,
		Date:          Date,
		API:           Compatibility,
		Compatibility: Compatibility,
	}
}
