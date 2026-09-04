package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
)

// fieldNames maps get_livedata_info field IDs to MQTT sub-topic names.
// IDs sourced from Ecowitt HTTP API Interface Protocol v1.0.5/v1.0.6.
// Decimal IDs ("3", "5") are derived/calculated values the gateway computes itself.
var fieldNames = map[string]string{
	// Temperature / humidity / pressure
	"0x02": "outdoor_temp",
	"0x03": "dew_point",
	"3":    "feels_like",
	"5":    "vapor_pressure_deficit",
	"0x07": "outdoor_humidity",
	// Wind
	"0x0A": "wind_direction",
	"0x0B": "wind_speed",
	"0x0C": "wind_gust",
	"0x19": "wind_speed_10min_avg",
	// Rain (tipping bucket)
	"0x0D": "rain_event",
	"0x0E": "rain_rate",
	"0x0F": "rain_hourly",
	"0x10": "rain_daily",
	"0x11": "rain_weekly",
	"0x12": "rain_monthly",
	"0x13": "rain_yearly",
	"0x14": "rain_total",
	"0x7C": "rain_event_total",
	"0x7D": "rain_hourly",
	// Solar / UV
	"0x15": "solar_radiation",
	"0x16": "uv_raw",
	"0x17": "uv_index",
	// Lightning (WS90)
	"0x60": "lightning_distance",
	"0x61": "lightning_count",
	"0x6D": "lightning_bearing",
}

type commonItem struct {
	ID   string `json:"id"`
	Val  string `json:"val"`
	Unit string `json:"unit"`
}

type wh25Item struct {
	InTemp string `json:"intemp"`
	Unit   string `json:"unit"`
	InHumi string `json:"inhumi"`
	Abs    string `json:"abs"`
	Rel    string `json:"rel"`
}

type piezoItem struct {
	ID      string `json:"id"`
	Val     string `json:"val"`
	Battery string `json:"battery"`
	Voltage string `json:"voltage"`
	CapVolt string `json:"ws90cap_volt"`
}

type liveDataResponse struct {
	CommonList []commonItem `json:"common_list"`
	WH25       []wh25Item   `json:"wh25"`
	Rain       []commonItem `json:"rain"`
	PiezoRain  []piezoItem  `json:"piezoRain"`
}

type config struct {
	gw1200Host   string
	mqttHost     string
	mqttPort     int
	mqttUser     string
	mqttPassword string
	mqttTopic    string
	pollInterval time.Duration
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func loadConfig() config {
	_ = godotenv.Load()

	pollSecs := 5
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pollSecs = n
		}
	}

	mqttPort := 1883
	if v := os.Getenv("MQTT_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mqttPort = n
		}
	}

	return config{
		gw1200Host:   os.Getenv("GW1200_HOST"),
		mqttHost:     os.Getenv("MQTT_HOST"),
		mqttPort:     mqttPort,
		mqttUser:     os.Getenv("MQTT_USER"),
		mqttPassword: os.Getenv("MQTT_PASSWORD"),
		mqttTopic:    os.Getenv("MQTT_TOPIC"),
		pollInterval: time.Duration(pollSecs) * time.Second,
	}
}

func fetchLiveData(host string) (*liveDataResponse, error) {
	resp, err := httpClient.Get(fmt.Sprintf("http://%s/get_livedata_info", host))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data liveDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func publishValue(client mqtt.Client, baseTopic, key, val string) {
	topic := baseTopic + "/" + key
	token := client.Publish(topic, 0, false, val)
	token.Wait()
	if err := token.Error(); err != nil {
		log.Printf("publish %s: %v", topic, err)
	}
}

type reading struct {
	Value         float64 `json:"value"`
	UnitOfMeasure string  `json:"unitOfMeasure"`
}

// parseReading extracts a numeric value and unit from a gateway value string.
// The unit may be embedded in val ("54%", "0.5 m/s") or supplied separately.
func parseReading(val, unit string) (reading, bool) {
	s := strings.TrimSpace(val)
	numStr, embedded := s, ""
	if i := strings.IndexByte(s, ' '); i >= 0 {
		numStr, embedded = s[:i], strings.TrimSpace(s[i+1:])
	} else if strings.HasSuffix(s, "%") {
		numStr, embedded = strings.TrimSuffix(s, "%"), "%"
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return reading{}, false
	}
	if embedded != "" {
		unit = embedded
	}
	return reading{Value: f, UnitOfMeasure: unit}, true
}

func buildReadings(data *liveDataResponse) map[string]reading {
	r := make(map[string]reading)

	add := func(name, val, unit string) {
		if rd, ok := parseReading(val, unit); ok {
			r[name] = rd
		}
	}

	for _, item := range append(data.CommonList, data.Rain...) {
		if name, ok := fieldNames[item.ID]; ok {
			add(name, item.Val, item.Unit)
		}
	}

	for _, wh := range data.WH25 {
		add("indoor_temp", wh.InTemp, wh.Unit)
		add("indoor_humidity", wh.InHumi, "")
		add("pressure_absolute", wh.Abs, "")
		add("pressure_relative", wh.Rel, "")
	}

	for _, item := range data.PiezoRain {
		if item.ID == "srain_piezo" {
			add("piezo_raining", item.Val, "")
			continue
		}
		if name, ok := fieldNames[item.ID]; ok {
			add("piezo_"+name, item.Val, "")
		}
		if item.Battery != "" {
			add("ws90_battery", item.Battery, "")
		}
		if item.Voltage != "" {
			add("ws90_voltage", item.Voltage, "V")
		}
		if item.CapVolt != "" {
			add("ws90_cap_volt", item.CapVolt, "V")
		}
	}

	return r
}

func publishJSON(client mqtt.Client, topic string, data *liveDataResponse) {
	payload, err := json.Marshal(buildReadings(data))
	if err != nil {
		log.Printf("json marshal: %v", err)
		return
	}
	publishValue(client, topic, "json", string(payload))
}

func poll(cfg config, client mqtt.Client) {
	data, err := fetchLiveData(cfg.gw1200Host)
	if err != nil {
		log.Printf("fetch error: %v", err)
		return
	}
	publishJSON(client, cfg.mqttTopic, data)
}

func main() {
	cfg := loadConfig()

	if cfg.gw1200Host == "" || cfg.mqttHost == "" || cfg.mqttTopic == "" {
		log.Fatal("GW1200_HOST, MQTT_HOST, and MQTT_TOPIC are required")
	}

	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:%d", cfg.mqttHost, cfg.mqttPort)).
		SetClientID("gw12002mqtt").
		SetUsername(cfg.mqttUser).
		SetPassword(cfg.mqttPassword).
		SetAutoReconnect(true).
		SetConnectTimeout(10 * time.Second)

	client := mqtt.NewClient(opts)
	log.Printf("connecting to MQTT broker at %s:%d", cfg.mqttHost, cfg.mqttPort)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT connect: %v", token.Error())
	}
	defer client.Disconnect(250)

	log.Printf("connected to MQTT broker at %s:%d", cfg.mqttHost, cfg.mqttPort)
	log.Printf("polling %s every %v, publishing to %s", cfg.gw1200Host, cfg.pollInterval, cfg.mqttTopic)

	poll(cfg, client)
	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		poll(cfg, client)
	}
}
