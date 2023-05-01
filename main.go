package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"context"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Config struct {
	VictoriaMetrics struct {
		Hostname string `json:"hostname"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"victoriaMetrics"`
	Servers []Server `json:"servers"`
}

type Sensor struct {
	Id int `json:"id"`
	SensorNumber int `json:"sensor_number"`
	Name string `json:"name"`
	OwnerId int `json:"owner_id"`
	OwnerLun int `json:"owner_lun"`
	RawReading float64 `json:"raw_reading"`
	Type string `json:"type"`
	TypeNumber int `json:"type_number"`
	Reading float64 `json:"reading"`
	SensorState int `json:"sensor_state"`
	DiscreteState int `json:"discrete_state"`
	SettableReadableThreshMask int `json:"settable_readable_threshMask"`
	LowerNonRecoverableThreshold float64 `json:"lower_non_recoverable_threshold"`
	LowerCriticalThreshold float64 `json:"lower_critical_threshold"`
	LowerNonCriticalThreshold float64 `json:"lower_non_critical_threshold"`
	HigherNonCriticalThreshold float64 `json:"higher_non_critical_threshold"`
	HigherCriticalThreshold float64 `json:"higher_critical_threshold"`
	HigherNonRecoverableThreshold float64 `json:"higher_non_recoverable_threshold"`
	Accessible int `json:"accessible"`
	Unit string `json:"unit"`
}

type VMRequest struct {
	Metric     map[string]string `json:"metric"`
	Values     []int64           `json:"values"`
	Timestamps []int64           `json:"timestamps"`
}

func main() {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	var config Config
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		log.Fatalf("Failed to decode JSON: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	for _, server := range config.Servers {
		go func(s Server) {
			session, csrfToken, err := CreateSession(s)
			if err != nil {
				log.Fatalf("Failed to create session: %v", err)
			}

			timer := time.NewTicker(30 * time.Second)

			for {
				select {
				case <-timer.C:
					req, err := http.NewRequest("GET", "https://" + s.Hostname + "/api/sensors", nil)
					if err != nil {
						log.Printf("Failed to create request: %v", err)
						continue
					}
					
					sessionCookie := &http.Cookie{
						Name:   "QSESSIONID",
						Domain: s.Hostname,
						Path:   "/",
						Value:  session,
						MaxAge: 0,
					}
					
					req.AddCookie(sessionCookie)
					req.Header.Set("X-CSRFTOKEN", csrfToken)

					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						log.Printf("Failed to send request: %v", err)
						continue
					}

					if resp.StatusCode != http.StatusOK {
						log.Printf("Request failed with status code %d", resp.StatusCode)
						continue
					}

					var sensors []Sensor
					err = json.NewDecoder(resp.Body).Decode(&sensors)
					if err != nil {
						log.Printf("Failed to decode JSON response: %v", err)
						continue
					}

					log.Printf("Successfully retrieved sensor data from %s", s.Hostname)
					resp.Body.Close()

					vmBody := ""

					for _, sensor := range sensors {
						if sensor.RawReading != 0 {
							values := &VMRequest{}
							values.Metric = map[string]string{
								"__name__": "asrockrack_"+strings.ToLower(strings.ReplaceAll(sensor.Name, " ", "_")),
								"hostname": s.Hostname,
								"unit": sensor.Unit,
							}
							
							values.Values = []int64{
								int64(sensor.Reading),
							}

							values.Timestamps = []int64{
								time.Now().UnixMilli(),
							}

							jsonMetrics, err := json.Marshal(values)
							if err != nil {
								log.Printf("Failed to marshal JSON: %v", err)
								continue
							}

							vmBody += string(jsonMetrics) + "\n"
						}
					}

					request, err := http.NewRequest("POST", config.VictoriaMetrics.Hostname+"/api/v1/import", bytes.NewBuffer([]byte(vmBody)))
					request.Header.Add("Authorization", "Basic "+BasicAuth(config.VictoriaMetrics.Username, config.VictoriaMetrics.Password))

					response, err := client.Do(request)
					if err != nil {
						log.Printf("Failed to send queue to VictoriaMetrics: %v", err)
					}

					err = response.Body.Close()
					if err != nil {
						log.Printf("Failed to close response body: %v", err)
					}

					log.Printf("Successfully sent queue to VictoriaMetrics for %s", s.Hostname)

				case <-ctx.Done():
					timer.Stop()
					return
				}
			}
		}(server)
	}

	<-sigChan
	cancel()
}
