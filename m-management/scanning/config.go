// Layer: APP · config. Reads the environment once, in cmd/.

package scanning

import (
	"time"

	"github.com/metap-org/metap/pkg/kit/env"
)

// Config is what scanning-service reads from its environment, once, in cmd/.
type Config struct {
	// ScannerURL is where a scan request is handed off. Empty (the default) means "no scanner
	// deployed": the job still queues, which is the honest state: something asked for a scan and
	// nothing is going to run it yet.
	ScannerURL string
}

// LoadConfig reads the settings: the environment, then the config file (CONFIG_FILE), then the default.
func LoadConfig() Config { return Config{ScannerURL: env.String("SCANNER_URL", "")} }

// scannerTimeout bounds the hand-off request to the scanner backend (the scan itself runs on).
const scannerTimeout = 15 * time.Second
