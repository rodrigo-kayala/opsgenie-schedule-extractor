package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
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

var holidays []string

func main() {
	apiKey := os.Args[1]
	scheduleName := os.Args[2]
	date := os.Args[3]
	holidays = strings.Split(os.Args[4], ",")

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

	var validHours map[string]time.Duration
	validHours = make(map[string]time.Duration)

	fmt.Println("\n====== schedule ======")
	for _, rotation := range response.Timeline.FinalSchedule.Rotations {
		for _, period := range rotation.Periods {
			if len(period.FlattenedRecipients) > 1 {
				panic("period with more than one recipient")
			}
			st := toTime(period.StartTime, loc)
			et := toTime(period.EndTime, loc)
			name := period.Recipients[0].DisplayName
			valid := calculateValidHours(st, et)
			fmt.Printf("%s : Start: %v End: %v Duration: %.2f ValidHours: %.2f\n",
				name,
				st,
				et,
				et.Sub(st).Hours(),
				valid.Hours(),
			)
			oncall[name] = oncall[name] + et.Sub(st)
			validHours[name] = validHours[name] + valid
		}
	}
	fmt.Println("\n====== on call time ======")
	for k, v := range oncall {
		fmt.Printf("%s: %.2f\n", k, v.Hours())
	}

	fmt.Println("\n====== valid on call time 19h - 07h ======")
	for k, v := range validHours {
		fmt.Printf("%s: %.2f\n", k, v.Hours())
	}
}
func singleDayHours(start time.Time, end time.Time) time.Duration {
	if !isWorkDay(start) {
		return end.Sub(start)
	}

	if start.Hour() >= 7 && start.Hour() < 19 {
		start = time.Date(start.Year(), start.Month(), start.Day(), 19, 0, 0, 0, start.Location())
	}

	if end.Hour() >= 7 && end.Hour() < 19 {
		end = time.Date(end.Year(), end.Month(), end.Day(), 7, 0, 0, 0, start.Location())
	}

	if (start.Hour() <= 7 && end.Hour() <= 7) || (start.Hour() >= 19 && end.Hour() >= 19) {
		return end.Sub(start)
	}
	ds := time.Date(start.Year(), start.Month(), start.Day(), 7, 0, 0, 0, start.Location())
	de := time.Date(end.Year(), end.Month(), end.Day(), 19, 0, 0, 0, start.Location())

	return ds.Sub(start) + end.Sub(de)
}

func calcFirstDay(start time.Time) time.Duration {
	c := time.Date(start.Year(), start.Month(), start.Day()+1, 0, 0, 0, 0, start.Location())

	if !isWorkDay(start) || start.Hour() >= 19 {
		return c.Sub(start)
	}

	as := time.Date(start.Year(), start.Month(), start.Day(), 7, 0, 0, 0, start.Location())
	ae := time.Date(start.Year(), start.Month(), start.Day(), 19, 0, 0, 0, start.Location())
	return as.Sub(start) + c.Sub(ae)
}

func calcLastDay(end time.Time) time.Duration {
	c := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())

	if !isWorkDay(end) || end.Hour() <= 7 {
		return end.Sub(c)
	}
	as := time.Date(end.Year(), end.Month(), end.Day(), 7, 0, 0, 0, end.Location())
	ae := time.Date(end.Year(), end.Month(), end.Day(), 19, 0, 0, 0, end.Location())
	return as.Sub(c) + end.Sub(ae)
}

func calculateValidHours(start time.Time, end time.Time) time.Duration {

	if start.Day() == end.Day() {
		return singleDayHours(start, end)
	}

	totalTime := calcFirstDay(start)
	c := time.Date(start.Year(), start.Month(), start.Day()+1, 0, 0, 0, 0, start.Location())

	for c.Day() < end.Day() {
		if isWorkDay(c) {
			totalTime = totalTime + (time.Hour * 12)
		} else {
			totalTime = totalTime + (time.Hour * 24)
		}
		c = c.AddDate(0, 0, 1)
	}

	totalTime = totalTime + calcLastDay(end)
	return totalTime
}

func toTime(t int64, loc *time.Location) time.Time {
	ret := time.Unix(t/1000, 000).In(loc)
	m, _ := time.ParseInLocation("2006-01-02", os.Args[3], loc)
	m = m.AddDate(0, 1, 0)
	if ret.After(m) || ret.Equal(m) {
		return m.Add(time.Nanosecond * -1.0)
	}
	return ret
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func isWorkDay(date time.Time) bool {
	return date.Weekday() != time.Sunday &&
		date.Weekday() != time.Saturday &&
		!contains(holidays, strconv.Itoa(date.Day()))
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
