// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/translate"
)

const weekMinutes = 7 * 24 * 60

// BroadcastSchedule is a compiled native weekly schedule for an explicit AP.
type BroadcastSchedule struct {
	Name, Target string
	Actions      []ScheduleAction
}

type ScheduleAction struct {
	Start        bool
	Hour, Minute int
	Days         []int // native numbering: Sunday=0
}

func (a ScheduleAction) String() string {
	kind := "stop"
	if a.Start {
		kind = "start"
	}
	var days []string
	for _, d := range a.Days {
		days = append(days, strconv.Itoa(d))
	}
	dow := strings.Join(days, ",")
	if len(a.Days) == 7 {
		dow = "*"
	}
	return fmt.Sprintf("action %s %d %d %s", kind, a.Minute, a.Hour, dow)
}

// ActiveAt evaluates the compiled event cycle at router-local civil time.
// It does not infer the router's timezone from the agent's host clock.
func (s BroadcastSchedule) ActiveAt(local time.Time) bool {
	minute := int(local.Weekday())*1440 + local.Hour()*60 + local.Minute()
	latest, active, found := 0, false, false
	for _, a := range s.Actions {
		for _, d := range a.Days {
			at := d*1440 + a.Hour*60 + a.Minute
			if at > minute {
				at -= weekMinutes
			}
			if !found || at > latest {
				latest, active, found = at, a.Start, true
			}
		}
	}
	return active
}

func scheduleMinute(value string, end bool) (int, error) {
	if len(value) != 4 {
		return 0, fmt.Errorf("invalid schedule time")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid schedule time")
		}
	}
	h, _ := strconv.Atoi(value[:2])
	m, _ := strconv.Atoi(value[2:])
	if end && h == 24 && m == 0 {
		return 1440, nil
	}
	if h > 23 || m > 59 {
		return 0, fmt.Errorf("schedule time is outside a civil day")
	}
	return h*60 + m, nil
}

func compileSchedule(cfg translate.Cfg, prefix, target string) (*BroadcastSchedule, error) {
	switch cfg[prefix+"schedule_enabled"] {
	case "", "disabled":
		return nil, nil
	case "enabled":
	default:
		return nil, fmt.Errorf("unknown WLAN schedule status")
	}
	if !regexpAP.MatchString(target) {
		return nil, fmt.Errorf("invalid scheduled AP target")
	}
	onDuring := false
	switch cfg[prefix+"schedule_invert"] {
	case "", "disabled":
	case "enabled":
		onDuring = true
	default:
		return nil, fmt.Errorf("unknown WLAN schedule direction")
	}
	dayNumber := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	var windows [weekMinutes]bool
	for key, value := range cfg {
		if !strings.HasPrefix(key, prefix+"schedule_") || key == prefix+"schedule_enabled" || key == prefix+"schedule_invert" {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(key, prefix+"schedule_"), ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid WLAN schedule window key")
		}
		day, exists := dayNumber[parts[0]]
		index, err := strconv.Atoi(parts[1])
		if !exists || err != nil || index < 0 || index >= 32 || strconv.Itoa(index) != parts[1] {
			return nil, fmt.Errorf("invalid WLAN schedule day or window index")
		}
		if len(value) != 9 || value[4] != '-' {
			return nil, fmt.Errorf("invalid WLAN schedule window")
		}
		start, err := scheduleMinute(value[:4], false)
		if err != nil {
			return nil, err
		}
		end, err := scheduleMinute(value[5:], true)
		if err != nil {
			return nil, err
		}
		length := end - start
		if length < 0 {
			length += 1440
		}
		for i := 0; i < length; i++ {
			windows[(day*1440+start+i)%weekMinutes] = true
		}
	}
	var states [weekMinutes]bool
	for i, inside := range windows {
		states[i] = inside == onDuring
	}
	type actionKey struct {
		start        bool
		hour, minute int
	}
	groups := map[actionKey][]int{}
	for i, active := range states {
		if active != states[(i+weekMinutes-1)%weekMinutes] {
			k := actionKey{active, (i % 1440) / 60, i % 60}
			groups[k] = append(groups[k], i/1440)
		}
	}
	if len(groups) == 0 {
		groups[actionKey{states[0], 0, 0}] = []int{0}
	}
	s := &BroadcastSchedule{Name: "UKN-" + strings.ReplaceAll(target, "/", "-"), Target: target}
	for k, days := range groups {
		s.Actions = append(s.Actions, ScheduleAction{Start: k.start, Hour: k.hour, Minute: k.minute, Days: days})
	}
	sort.Slice(s.Actions, func(i, j int) bool {
		a, b := s.Actions[i], s.Actions[j]
		if a.Days[0] != b.Days[0] {
			return a.Days[0] < b.Days[0]
		}
		if a.Hour != b.Hour {
			return a.Hour < b.Hour
		}
		return a.Minute < b.Minute
	})
	return s, nil
}
