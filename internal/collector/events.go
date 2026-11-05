package collector

import (
	"fmt"
	"iter"
	"net/url"
	"strings"
	"time"

	"github.com/mrlhansen/idrac_exporter/internal/config"
)

// UniversalSELEndpoints contains well-known System Event Log (SEL) Redfish endpoints
// covering Dell (iDRAC 7/8/9), Fujitsu (iRMC S4/S5/S6), HPE (iLO 4/5/6 IML), Lenovo (XCC), and Supermicro.
var UniversalSELEndpoints = []string{
	// 1. Dell iDRAC 8 (R730, R630, T630)
	"/redfish/v1/Managers/iDRAC.Embedded.1/Logs/Sel",
	// 2. Dell iDRAC 9 (R740, R640, R750, R760) & Standard DMTF
	"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Sel/Entries",
	"/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/SEL/Entries",
	"/redfish/v1/Systems/System.Embedded.1/LogServices/Sel/Entries",
	"/redfish/v1/Systems/System.Embedded.1/LogServices/SEL/Entries",
	// 3. Fujitsu iRMC (S4, S5, S6)
	"/redfish/v1/Systems/0/LogServices/SEL/Entries",
	"/redfish/v1/Managers/0/LogServices/SEL/Entries",
	"/redfish/v1/Systems/1/LogServices/SEL/Entries",
	"/redfish/v1/Managers/iRMC/LogServices/SystemEventLog/Entries",
	// 4. HPE iLO 4, 5, 6 (Integrated Management Log / IML)
	"/redfish/v1/Systems/1/LogServices/IML/Entries",
	"/redfish/v1/Systems/1/LogServices/EventLog/Entries",
	// 5. Lenovo XCC / Supermicro BMC
	"/redfish/v1/Managers/1/LogServices/SEL/Entries",
	"/redfish/v1/Systems/1/LogServices/PlatformLog/Entries",
	"/redfish/v1/Systems/1/LogServices/StandardLog/Entries",
	"/redfish/v1/Systems/1/LogServices/Log1/Entries",
}

type EventLog struct {
	config   *config.EventConfig
	response *EventLogResponse
}

type Event struct {
	Id        string
	Message   string
	Severity  string
	Timestamp time.Time
}

func parseEventTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.DateTime,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}

func (client *Client) QueryEventLog(path string, config *config.EventConfig) (*EventLog, bool) {
	if !config.Enabled {
		return nil, true
	}

	pathsToTry := []string{}
	if path != "" {
		pathsToTry = append(pathsToTry, path)
	} else {
		pathsToTry = append(pathsToTry, UniversalSELEndpoints...)
	}

	for _, p := range pathsToTry {
		queryPath := p
		if client.features.Filter {
			filter := url.Values{}

			switch config.SeverityLevel {
			case 1:
				filter.Set("$filter", "Severity eq 'Critical' or Severity eq 'Warning'")
			case 2:
				filter.Set("$filter", "Severity eq 'Critical'")
			}

			if s := filter.Encode(); len(s) > 0 {
				queryPath = queryPath + "?" + s
			}
		}

		resp := &EventLogResponse{}
		ok := client.redfish.Get(queryPath, resp)
		if ok && resp != nil {
			if client.path.Event == "" {
				client.path.Event = p
			}
			return &EventLog{
				config:   config,
				response: resp,
			}, true
		}
	}

	return nil, false
}

func (e *EventLog) Items() iter.Seq[Event] {
	return func(yield func(Event) bool) {

		// Some machines will report filtering as a supported feature, but in reality the
		// returned data is not always properly filtered. For this reason we always filter
		// the data when looping through the entries.

		for _, v := range e.response.Members {
			t, err := parseEventTime(v.Created)
			if err != nil {
				continue
			}

			d := time.Since(t)
			if d.Seconds() > e.config.MaxAgeSeconds {
				continue
			}

			severity := health2value(v.Severity)
			if severity < e.config.SeverityLevel {
				continue
			}

			v := Event{
				Id:        v.Id,
				Message:   strings.TrimSpace(v.Message),
				Severity:  v.Severity,
				Timestamp: t,
			}

			if !yield(v) {
				return
			}
		}
	}
}

// LatestWarningOrCritical returns the most recent Warning or Critical SEL event.
func (e *EventLog) LatestWarningOrCritical() *Event {
	if e == nil || e.response == nil || len(e.response.Members) == 0 {
		return nil
	}

	var latest *Event

	for _, m := range e.response.Members {
		sevLower := strings.ToLower(m.Severity)
		if sevLower != "critical" && sevLower != "warning" {
			continue
		}

		t, _ := parseEventTime(m.Created)

		// Clean message string (replace newlines and carriage returns with space)
		cleanMsg := strings.ReplaceAll(m.Message, "\r\n", " ")
		cleanMsg = strings.ReplaceAll(cleanMsg, "\n", " ")
		cleanMsg = strings.ReplaceAll(cleanMsg, "\r", " ")
		cleanMsg = strings.TrimSpace(cleanMsg)
		if cleanMsg == "" {
			cleanMsg = "Unknown event"
		}

		ev := &Event{
			Id:        m.Id,
			Message:   cleanMsg,
			Severity:  m.Severity,
			Timestamp: t,
		}

		if latest == nil {
			latest = ev
		} else if !t.IsZero() && !latest.Timestamp.IsZero() {
			if t.After(latest.Timestamp) {
				latest = ev
			}
		} else {
			// Fallback string compare when parse fails
			if m.Created > latest.Timestamp.Format(time.RFC3339) {
				latest = ev
			}
		}
	}

	return latest
}
