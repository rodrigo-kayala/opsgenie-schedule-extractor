package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"
)

type timelineResponse struct {
	Schedule *schedule `json:"schedule"`
	Took     int64     `json:"took"`
	Timeline timeline  `json:"timeline"`
}

type timeline struct {
	StartTime     int64          `json:"startTime"`
	EndTime       int64          `json:"endTime"`
	FinalSchedule *finalSchedule `json:"finalSchedule"`
}

type finalSchedule struct {
	Rotations []rotation `json:"rotations"`
}

type rotation struct {
	Name    string   `json:"name"`
	ID      string   `json:"id"`
	Order   float64  `json:"order"`
	Periods []period `json:"periods"`
}

type period struct {
	StartTime           int64       `json:"startTime"`
	EndTime             int64       `json:"endTime"`
	Type                string      `json:"type"`
	FlattenedRecipients []recipient `json:"flattenedRecipients"`
	Recipients          []recipient `json:"recipients"`
}

type recipient struct {
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	ID          string `json:"id"`
	Type        string `json:"type"`
}

type schedule struct {
	Timezone string `json:"timezone"`
	Name     string `json:"name"`
	ID       string `json:"id"`
	Team     string `json:"team"`
	Enabled  bool   `json:"enabled"`
}

const (
	opsGenieTimelineEndpoit = "https://api.opsgenie.com/v1/json/schedule/timeline"
)

func main() {
	apiKey := os.Args[1]
	scheduleName := os.Args[2]
	date := os.Args[3]

	u := &url.URL{
		Scheme: "https",
		Host:   "api.opsgenie.com",
		Path:   "/v1/json/schedule/timeline",
	}
	q := u.Query()
	q.Set("apiKey", apiKey)
	q.Set("name", scheduleName)
	q.Set("intervalUnit", "months")
	q.Set("date", date+" 00:00")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	check(err)

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	check(err)

	t := timelineResponse{}
	err = json.Unmarshal(data, &t)
	check(err)

	printReport(&t)
}

func printReport(response *timelineResponse) {
	fmt.Printf("Team: %s\n", response.Schedule.Team)
	fmt.Printf("Schedule: %s\n", response.Schedule.Name)

	loc, err := time.LoadLocation(response.Schedule.Timezone)
	check(err)

	fmt.Printf("Start time: %v\n", toTime(response.Timeline.StartTime, loc))
	fmt.Printf("End time: %v\n", toTime(response.Timeline.EndTime, loc))

	var oncall map[string]time.Duration
	oncall = make(map[string]time.Duration)

	fmt.Println("\n====== schedule ======")
	for _, rotation := range response.Timeline.FinalSchedule.Rotations {
		for _, period := range rotation.Periods {
			if len(period.FlattenedRecipients) > 1 {
				panic("period with more than one recipient")
			}
			st := toTime(period.StartTime, loc)
			et := toTime(period.EndTime, loc)
			name := period.Recipients[0].DisplayName
			fmt.Printf("%s : Start: %v End: %v Duration: %v\n",
				name,
				st,
				et,
				et.Sub(st).Hours(),
			)
			oncall[name] = oncall[name] + et.Sub(st)
		}
	}
	fmt.Println("\n====== on call time ======")
	for k, v := range oncall {
		fmt.Printf("%s: %v\n", k, v.Hours())
	}
}

func toTime(t int64, loc *time.Location) time.Time {
	ret := time.Unix(t/1000, 000).In(loc)
	m, _ := time.ParseInLocation("2006-01-02", os.Args[3], loc)
	m = m.AddDate(0, 1, 0)
	if ret.After(m) {
		return m
	}
	return ret
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
