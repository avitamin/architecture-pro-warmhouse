package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func measure(t *testing.T, api *temperatureAPI, path string) temperatureResponse {
	t.Helper()
	start := time.Now().UTC()
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	var result temperatureResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 8 {
		t.Fatalf("unexpected contract: %v", fields)
	}
	for _, name := range []string{"unit", "timestamp", "location", "status", "sensor_id", "sensor_type", "description"} {
		if _, ok := fields[name].(string); !ok {
			t.Fatalf("%s must be string: %v", name, fields[name])
		}
	}
	if _, ok := fields["value"].(float64); !ok {
		t.Fatal("value must be number")
	}
	if result.Value < 15 || result.Value > 30 || math.Abs(result.Value*10-math.Round(result.Value*10)) > 1e-9 {
		t.Fatalf("invalid temperature %v", result.Value)
	}
	if result.Unit != "°C" || result.Status != "active" || result.SensorType != "temperature" || result.Description == "" {
		t.Fatalf("invalid response: %+v", result)
	}
	if result.Timestamp.Before(start.Add(-time.Second)) || result.Timestamp.After(time.Now().Add(time.Second)) || result.Timestamp.Location() != time.UTC {
		t.Fatalf("invalid timestamp %v", result.Timestamp)
	}
	return result
}

func TestTemperatureContract(t *testing.T) {
	api := newTemperatureAPI()
	for _, tc := range []struct{ path, id, location string }{
		{"/temperature/1", "1", "Living Room"},
		{"/temperature/2", "2", "Bedroom"},
		{"/temperature/3", "3", "Kitchen"},
		{"/temperature?location=Living%20Room", "1", "Living Room"},
		{"/temperature?location=Bedroom", "2", "Bedroom"},
		{"/temperature?location=Kitchen", "3", "Kitchen"},
		{"/temperature/99", "99", "Unknown"},
		{"/temperature/custom", "custom", "Unknown"},
		{"/temperature?location=Office", "0", "Office"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := measure(t, api, tc.path)
			if got.SensorID != tc.id || got.Location != tc.location {
				t.Fatalf("wrong mapping: %+v", got)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	api := newTemperatureAPI()
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/temperature", 400}, {"GET", "/temperature?location=", 400},
		{"GET", "/temperature?location=%20%09", 400},
		{"GET", "/temperature/", 404}, {"GET", "/temperature/1/extra", 404},
		{"GET", "/missing", 404}, {"POST", "/missing", 404},
		{"POST", "/temperature?location=Kitchen", 405}, {"PUT", "/temperature/1", 405},
		{"HEAD", "/health", 405}, {"POST", "/health", 405},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.code {
				t.Fatalf("got %d, want %d", w.Code, tc.code)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == "" {
				t.Fatalf("invalid error: %s", w.Body.String())
			}
			if tc.code == 405 && w.Header().Get("Allow") != "GET" {
				t.Fatal("missing Allow")
			}
			if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatal("wrong content type")
			}
		})
	}
}

func TestHistoryAndNoAdjacentRepeats(t *testing.T) {
	api := newTemperatureAPI()
	var previous float64
	for i := 0; i < 1000; i++ {
		path := "/temperature/1"
		if i%2 == 1 {
			path = "/temperature?location=Living%20Room"
		}
		got := measure(t, api, path)
		if i > 0 && got.Value == previous {
			t.Fatalf("adjacent repeat at %d", i)
		}
		previous = got.Value
	}
	if len(api.last) != 1 {
		t.Fatalf("ID/location history must be shared: %v", api.last)
	}
	// Force both bounds as preceding values; neither can repeat.
	for _, bound := range []int{150, 300} {
		api.last["id:1"] = bound
		if got := measure(t, api, "/temperature/1"); got.Value == float64(bound)/10 {
			t.Fatalf("bound repeated: %v", got.Value)
		}
	}
	measure(t, api, "/temperature/99")
	measure(t, api, "/temperature?location=Office")
	measure(t, api, "/temperature?location=Garage")
	if len(api.last) != 4 {
		t.Fatalf("unknown sensors need separate history: %v", api.last)
	}
	before := make(map[string]int)
	for k, v := range api.last {
		before[k] = v
	}
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 200 || w.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("health: %s", w.Body.String())
	}
	if !reflect.DeepEqual(before, api.last) {
		t.Fatal("health changed history")
	}
}

func TestConcurrentRequests(t *testing.T) {
	api := newTemperatureAPI()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				path := fmt.Sprintf("/temperature/%d", i%3+1)
				if j%2 == 0 {
					path = "/temperature?location=Living%20Room"
				}
				measure(t, api, path)
			}
		}(i)
	}
	wg.Wait()
	if len(api.last) != 3 {
		t.Fatalf("unexpected sensors: %v", api.last)
	}
}
