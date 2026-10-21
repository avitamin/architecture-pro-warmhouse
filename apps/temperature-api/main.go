package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var locations = map[string]string{"1": "Living Room", "2": "Bedroom", "3": "Kitchen"}

type temperatureResponse struct {
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Timestamp   time.Time `json:"timestamp"`
	Location    string    `json:"location"`
	Status      string    `json:"status"`
	SensorID    string    `json:"sensor_id"`
	SensorType  string    `json:"sensor_type"`
	Description string    `json:"description"`
}

type temperatureAPI struct {
	mu   sync.Mutex
	last map[string]int
}

func newTemperatureAPI() *temperatureAPI {
	return &temperatureAPI{last: make(map[string]int)}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

func (api *temperatureAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	byLocation := path == "/temperature"
	byID := strings.HasPrefix(path, "/temperature/") && len(path) > len("/temperature/") && !strings.Contains(strings.TrimPrefix(path, "/temperature/"), "/")
	if !byLocation && !byID && path != "/health" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Route not found"})
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	if path == "/health" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	var id, location, key string
	if byLocation {
		location = r.URL.Query().Get("location")
		if strings.TrimSpace(location) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Location is required"})
			return
		}
		id = "0"
		key = "location:" + location
		for knownID, knownLocation := range locations {
			if location == knownLocation {
				id = knownID
				key = "id:" + id
				break
			}
		}
	} else {
		id = strings.TrimPrefix(path, "/temperature/")
		key = "id:" + id
		location = locations[id]
		if location == "" {
			location = "Unknown"
		}
	}
	// Generate under the same lock as history updates. Delivery order of
	// concurrent HTTP responses is independent of this generation order.
	api.mu.Lock()
	value := rand.Intn(151) + 150
	if previous, ok := api.last[key]; ok {
		value = rand.Intn(150) + 150
		if value >= previous {
			value++
		}
	}
	api.last[key] = value
	api.mu.Unlock()
	writeJSON(w, http.StatusOK, temperatureResponse{
		Value: float64(value) / 10, Unit: "°C", Timestamp: time.Now().UTC(),
		Location: location, Status: "active", SensorID: id, SensorType: "temperature",
		Description: "Emulated temperature measurement",
	})
}

func main() {
	server := &http.Server{Addr: ":8081", Handler: newTemperatureAPI(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("Temperature API listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
