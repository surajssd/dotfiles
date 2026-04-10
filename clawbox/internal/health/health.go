package health

import (
	"fmt"
	"net/http"
	"time"
)

const (
	maxAttempts  = 30
	pollInterval = 2 * time.Second
	httpTimeout  = 2 * time.Second
)

// WaitForHealthy polls the gateway health endpoint until it responds.
// It prints progress dots and is non-fatal on timeout.
func WaitForHealthy(sessionName, containerName string, gatewayPort int) {
	waitForHealthyWithConfig(sessionName, containerName, gatewayPort, maxAttempts, pollInterval, httpTimeout)
}

func waitForHealthyWithConfig(sessionName, containerName string, gatewayPort, attempts int, interval, timeout time.Duration) {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", gatewayPort)
	client := &http.Client{Timeout: timeout}

	fmt.Print("⏳ Waiting for gateway to become healthy ")

	for range attempts {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Println()
				fmt.Printf("✅ Container '%s' is running and healthy.\n", containerName)
				return
			}
		}

		fmt.Print(".")
		time.Sleep(interval)
	}

	fmt.Println()
	fmt.Printf("⚠️  Gateway did not become healthy within %ds.\n", attempts*int(interval.Seconds()))
	fmt.Println("📋 Check logs for errors:")
	fmt.Printf("  clawbox logs %s\n", sessionName)
}
