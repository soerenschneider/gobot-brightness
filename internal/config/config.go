package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

const (
	BotName                = "gobot_lux"
	defaultLogSensor       = false
	defaultIntervalSeconds = 30
	defaultMetricConfig    = ":9194"
	maxStatsBucketSeconds  = 7200
)

var (
	defaultStatsBucketsSeconds = []int{15, 30, 60, 120, 300, 600, 1800}
	once                       sync.Once
	validate                   *validator.Validate
)

type Config struct {
	Placement     string `json:"placement,omitempty" validate:"required"`
	MetricConfig  string `json:"metrics_addr,omitempty" validate:"omitempty,tcp_addr"`
	IntervalSecs  int    `json:"interval_s,omitempty" validate:"min=1,max=300"`
	StatIntervals []int  `json:"stat_intervals,omitempty" validate:"dive,min=10,max=3600"`
	LogSensor     bool   `json:"log_sensor,omitempty"`
	MqttConfig
	SensorConfig
}

func DefaultConfig() Config {
	return Config{
		LogSensor:     defaultLogSensor,
		IntervalSecs:  defaultIntervalSeconds,
		MetricConfig:  defaultMetricConfig,
		StatIntervals: defaultStatsBucketsSeconds,
		SensorConfig:  defaultSensorConfig(),
	}
}

func ConfigFromEnv() Config {
	conf := DefaultConfig()

	location, err := fromEnv("LOCATION")
	if err == nil {
		conf.Placement = location
	}

	logValues, err := fromEnvBool("LOG_SENSOR")
	if err == nil {
		conf.LogSensor = logValues
	}

	intervalSeconds, err := fromEnvInt("INTERVAL_S")
	if err == nil {
		conf.IntervalSecs = intervalSeconds
	}

	mqttHost, err := fromEnv("MQTT_HOST")
	if err == nil {
		conf.Host = mqttHost
	}

	mqttTopic, err := fromEnv("MQTT_TOPIC")
	if err == nil {
		conf.Topic = mqttTopic
	}

	mqttStatsTopic, err := fromEnv("MQTT_STATS_TOPIC")
	if err == nil {
		conf.StatsTopic = mqttStatsTopic
	}

	metricConfig, err := fromEnv("METRICS_ADDR")
	if err == nil {
		conf.MetricConfig = metricConfig
	}

	clientKeyFile, err := fromEnv("SSL_CLIENT_KEY_FILE")
	if err == nil {
		conf.ClientKeyFile = clientKeyFile
	}

	clientCertFile, err := fromEnv("SSL_CLIENT_CERT_FILE")
	if err == nil {
		conf.ClientCertFile = clientCertFile
	}

	conf.SensorConfig.ConfigFromEnv()

	return conf
}

func ReadJsonConfig(filePath string) (*Config, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read config from file: %v", err)
	}

	ret := DefaultConfig()
	err = json.Unmarshal(fileContent, &ret)
	return &ret, err
}

func (conf *Config) Validate() error {
	once.Do(func() {
		validate = validator.New()
		if err := validate.RegisterValidation("mqtt_topic", validateTopic); err != nil {
			log.Fatal("could not build custom validation 'mqtt_topic'")
		}
		if err := validate.RegisterValidation("mqtt_broker", validateBroker); err != nil {
			log.Fatal("could not build custom validation 'validateBroker'")
		}
	})
	return validate.Struct(conf)
}

func validateTopic(fl validator.FieldLevel) bool {
	// Get the field value and check if it's a slice
	field := fl.Field()
	if field.Kind() != reflect.String {
		return false
	}

	topic, ok := field.Interface().(string)
	if !ok || !matchTopic(topic) {
		return false
	}

	return true
}

func validateBroker(fl validator.FieldLevel) bool {
	// Get the field value and check if it's a slice
	field := fl.Field()
	if field.Kind() != reflect.String {
		return false
	}

	// Convert to string and check its value
	broker, ok := field.Interface().(string)
	if !ok || !matchHost(broker) {
		return false
	}

	return true
}

func (conf *Config) Validat2e() error {
	if conf.Placement == "" {
		return errors.New("empty location provided")
	}

	if conf.IntervalSecs < 1 {
		return fmt.Errorf("invalid interval: must not be lower than 1 but is %d", conf.IntervalSecs)
	}

	if conf.IntervalSecs < conf.AioPollingIntervalMs/1000 {
		return fmt.Errorf("invalid interval: must not be lower than aioPollingIntervalMs (%ds): %d", conf.AioPollingIntervalMs/1000, conf.IntervalSecs)
	}

	if conf.IntervalSecs > 300 {
		return fmt.Errorf("invalid interval: mut not be greater than 300 but is %d", conf.IntervalSecs)
	}

	if err := conf.SensorConfig.Validate(); err != nil {
		return err
	}

	if len(conf.StatIntervals) > 0 {
		min, _ := conf.GetStatIntervalMin()
		if min < 1 {
			return fmt.Errorf("minimal value in stats bucket must not be < 1: %d", min)
		}

		max, _ := conf.GetStatIntervalMax()
		if max > maxStatsBucketSeconds {
			return fmt.Errorf("maximal value in stats bucket must not be > %d: %d", maxStatsBucketSeconds, max)
		}
	}

	return nil
}

func (conf *Config) Print() {
	log.Println("-----------------")
	log.Println("Configuration:")
	log.Printf("Placement=%s", conf.Placement)
	log.Printf("LogSensor=%t", conf.LogSensor)
	log.Printf("MetricConfig=%s", conf.MetricConfig)
	log.Printf("IntervalSecs=%d", conf.IntervalSecs)
	log.Printf("Host=%s", conf.Host)
	log.Printf("Topic=%s", conf.Topic)
	if len(conf.MqttConfig.StatsTopic) > 0 {
		log.Printf("StatsTopic=%s", conf.Topic)
	}
	if len(conf.StatIntervals) > 0 {
		log.Printf("StatIntervals=%v", conf.StatIntervals)
	}

	conf.SensorConfig.Print()

	log.Println("-----------------")
}

func computeEnvName(name string) string {
	return fmt.Sprintf("%s_%s", strings.ToUpper(BotName), strings.ToUpper(name))
}

func fromEnv(name string) (string, error) {
	name = computeEnvName(name)
	val := os.Getenv(name)
	if val == "" {
		return "", errors.New("not defined")
	}
	return val, nil
}

func fromEnvInt(name string) (int, error) {
	val, err := fromEnv(name)
	if err != nil {
		return -1, err
	}

	parsed, err := strconv.Atoi(val)
	if err != nil {
		return -1, err
	}
	return parsed, nil
}

func fromEnvBool(name string) (bool, error) {
	val, err := fromEnv(name)
	if err != nil {
		return false, err
	}

	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return false, err
	}
	return parsed, nil
}

func (conf *Config) GetStatIntervalMin() (int, error) {
	if len(conf.StatIntervals) == 0 {
		return -1, fmt.Errorf("empty array provided")
	}

	min := conf.StatIntervals[0]
	for _, val := range conf.StatIntervals {
		if val < min {
			min = val
		}
	}

	return min, nil
}

func (conf *Config) GetStatIntervalMax() (int, error) {
	if len(conf.StatIntervals) == 0 {
		return -1, fmt.Errorf("empty array provided")
	}

	max := conf.StatIntervals[0]
	for _, val := range conf.StatIntervals {
		if val > max {
			max = val
		}
	}

	return max, nil
}
