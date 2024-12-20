package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Result struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Date         string `json:"date"`
	Expanded     bool   `json:"expanded"`
	LocationType string `json:"location_type"`
	LocationName string `json:"location_name"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Payload struct {
	Attributes struct {
		Message      string   `json:"message"`
		Priority     int      `json:"priority"`
		Service      string   `json:"service"`
		Source       string   `json:"source"`
		Status       int      `json:"status"`
		Type         int      `json:"type"`
		Environment  int      `json:"environment"`
		Impact       bool     `json:"impact"`
		StartDate    string   `json:"start_date"`
		EndDate      string   `json:"end_date"`
		Owner        string   `json:"owner"`
		StackHolders []string `json:"stackHolders"`
		Notification bool     `json:"notification"`
	} `json:"attributes"`
	Links struct {
		PullRequestLink string `json:"pull_request_link"`
		Ticket          string `json:"ticket"`
	} `json:"links"`
	Title   string `json:"title"`
	SlackId string `json:"slack_id"`
}

type Config struct {
	CheckInverval      int
	TrackerEnv         int
	TrackerHost        string
	TrackerService     string
	TrackerOwner       string
	TrackerSource      string
	ProductPlanHost    string
	ProductPlanRoadmap string
	ProductPlanToken   string
}

var ConfigGeneral = Config{
	CheckInverval:   300,
	ProductPlanHost: "app.productplan.com",
	TrackerOwner:    "ProductPlan",
	TrackerSource:   "ProductPlan",
	TrackerEnv:      7,
}

func main() {
	// Initialisation synchronisation of events
	slog.Info(
		"init synchronize events from productplan",
	)
	synchronizeEvents()
	// Create a new ticker which will fire every check inverval
	ticker := time.NewTicker(time.Duration(ConfigGeneral.CheckInverval) * time.Second)
	defer ticker.Stop() // S'assure que le ticker est arrêté proprement

	go func() {
		for {
			t, ok := <-ticker.C
			if !ok {
				break
			}
			slog.Info(
				"synchronize events from productplan",
				"time", t.Format("15:04:05"),
			)
			synchronizeEvents()
		}
	}()

	// Autres tâches dans le programme principal
	select {} // Bloque le programme principal indéfiniment
}

func createPayload(milestones Result) Payload {
	var data Payload

	data.Attributes.Message = milestones.Description
	data.Attributes.Priority = 1
	data.Attributes.Service = ConfigGeneral.TrackerService
	data.Attributes.Source = ConfigGeneral.TrackerSource
	data.Attributes.Status = 1
	data.Attributes.Type = 1
	data.Attributes.Environment = ConfigGeneral.TrackerEnv
	data.Attributes.Impact = false
	data.Attributes.StartDate = ParsedTime(fmt.Sprintf("%sT09:00", milestones.Date)).Format("2006-01-02T15:04:05Z") //time.Unix(tracker.Datetime, 0).Format("2006-01-02T15:04:05Z")
	data.Attributes.EndDate = ParsedTime(fmt.Sprintf("%sT18:00", milestones.Date)).Format("2006-01-02T15:04:05Z")   //time.Unix(tracker.EndDate, 0).Format("2006-01-02T15:04:05Z")
	data.Attributes.Owner = ConfigGeneral.TrackerOwner
	data.Title = milestones.Title
	data.SlackId = strconv.Itoa(milestones.ID)

	return data
}

func synchronizeEvents() {

	url := fmt.Sprintf("https://%s/api/v2/roadmaps/%s/milestones", ConfigGeneral.ProductPlanHost, ConfigGeneral.ProductPlanRoadmap)

	req, _ := http.NewRequest("GET", url, nil)

	token := fmt.Sprintf("Bearer %s", ConfigGeneral.ProductPlanToken)
	req.Header.Add("accept", "application/json")
	req.Header.Add("authorization", token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)

	}
	defer func() {
		err := res.Body.Close()
		if err != nil {
			panic(err)
		}
	}()

	if res.StatusCode != 200 {
		slog.Error(
			"error to get productplan  milestones",
			"statusCode", res.Status)
		panic(res.Status)
	}

	body, _ := io.ReadAll(res.Body)

	var data struct {
		Results []Result `json:"results"`
	}

	err = json.Unmarshal([]byte(body), &data)
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return
	}

	// Iterate over results and update request
	for _, Results := range data.Results {
		payload := createPayload(Results)
		if getTrackerEvent(strconv.Itoa(Results.ID)) {
			slog.Info(
				"event already exist",
				"event", payload.Title,
				"slack_id", payload.SlackId,
			)
			updateTrackerEvent(payload)
			continue
		}
		slog.Info(
			"event creating",
			"event", payload.Title,
			"slack_id", payload.SlackId,
		)
		postTrackerEvent(payload)
	}
}

func postTrackerEvent(payload Payload) {

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		ErrAttr(err)
	}

	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", ConfigGeneral.TrackerHost+"/api/v1alpha1/event", body)
	if err != nil {
		ErrAttr(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ErrAttr(err)
	}
	defer resp.Body.Close()

	slog.Info(
		"event created",
		"event", payload.Title,
		"slack_id", payload.SlackId,
	)
}

func getTrackerEvent(id string) bool {
	req, _ := http.NewRequest("GET", ConfigGeneral.TrackerHost+"/api/v1alpha1/event/"+id, nil)
	req.Header.Add("accept", "application/json")

	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()

	return res.StatusCode == 200
}

func updateTrackerEvent(payload Payload) {

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		ErrAttr(err)
	}

	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("PUT", ConfigGeneral.TrackerHost+"/api/v1alpha1/event", body)
	if err != nil {
		ErrAttr(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ErrAttr(err)
	}
	defer resp.Body.Close()

	slog.Info(
		"event updated",
		"event", payload.Title,
		"slack_id", payload.SlackId,
	)
}

func ParsedTime(dateStr string) time.Time {

	// Parsing de la chaîne en time.Time
	layout := "2006-01-02T15:04" // Format attendu
	parsedTime, err := time.Parse(layout, dateStr)
	if err != nil {
		fmt.Println("Erreur lors du parsing:", err)
		return parsedTime
	}
	return parsedTime
}

func ErrAttr(err error) slog.Attr {
	return slog.Any("error", err)
}

func init() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if os.Getenv("TRACKER_HOST") != "" {
		ConfigGeneral.TrackerHost = os.Getenv("TRACKER_HOST")
	}
	if os.Getenv("CHECK_INTERVAL") != "" {
		interval, err := strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
		if err != nil {
			fmt.Println("Erreur :", err)
			return
		}
		ConfigGeneral.CheckInverval = interval
	}
	if os.Getenv("TRACKER_SERVICE") != "" {
		ConfigGeneral.TrackerService = os.Getenv("TRACKER_SERVICE")
	}
	if os.Getenv("TRACKER_OWNER") != "" {
		ConfigGeneral.TrackerOwner = os.Getenv("TRACKER_OWNER")
	}
	if os.Getenv("TRACKER_SOURCE") != "" {
		ConfigGeneral.TrackerSource = os.Getenv("TRACKER_SOURCE")
	}
	if os.Getenv("PRODUCTPLAN_HOST") != "" {
		ConfigGeneral.ProductPlanHost = os.Getenv("PRODUCTPLAN_HOST")
	}
	if os.Getenv("PRODUCTPLAN_ROADMAP") != "" {
		ConfigGeneral.ProductPlanRoadmap = os.Getenv("PRODUCTPLAN_ROADMAP")
	}
	if os.Getenv("PRODUCTPLAN_TOKEN") != "" {
		ConfigGeneral.ProductPlanToken = os.Getenv("PRODUCTPLAN_TOKEN")
	}

}
