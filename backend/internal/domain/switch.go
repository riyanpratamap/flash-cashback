package domain

import "fmt"

// Switch names one of the two operator switches on the campaign row (D05).
// The set is closed: the store picks a column from it, never from input.
type Switch string

// The two switches.
const (
	SwitchAwards      Switch = "awards"
	SwitchRedemptions Switch = "redemptions"
)

// Valid reports whether s is one of the two switches.
func (s Switch) Valid() bool { return s == SwitchAwards || s == SwitchRedemptions }

// Action is what an operator command does to a switch: "pause" or "resume".
func Action(paused bool) string {
	if paused {
		return "pause"
	}
	return "resume"
}

// ParseSwitchCommand maps a command name such as "pause-awards" to its
// switch and target value (paused true for pause).
func ParseSwitchCommand(name string) (sw Switch, paused bool, err error) {
	switch name {
	case "pause-awards":
		return SwitchAwards, true, nil
	case "resume-awards":
		return SwitchAwards, false, nil
	case "pause-redemptions":
		return SwitchRedemptions, true, nil
	case "resume-redemptions":
		return SwitchRedemptions, false, nil
	}
	return "", false, fmt.Errorf("unknown command %q", name)
}
