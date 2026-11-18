package dice

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// Roll is one independent result of a dice notation.
type Roll struct {
	Index    int    `json:"index"`
	Notation string `json:"notation"`
	Dice     []int  `json:"dice"`
	Modifier int    `json:"modifier"`
	Total    int    `json:"total"`
}

// Result is a batch of rolls from the dice upstream.
type Result struct {
	Rolls    []Roll
	Seed     int64
	Count    int
	Notation string
}

// Request is a validated roll shared by HTTP and gRPC.
type Request struct {
	Notation string
	Count    int
	Seed     *int64
}

// Roller rolls dice. The HTTP gateway depends on this interface;
// the gRPC client is the production implementation.
type Roller interface {
	Roll(ctx context.Context, req Request) (Result, error)
}

var notationRE = regexp.MustCompile(`^(?:([1-9]\d{0,2})?d([1-9]\d{0,3}))([+-]\d{1,4})?$`)

// ValidationError is a caller mistake, mapped to HTTP 400 and gRPC InvalidArgument.
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }

func invalid(msg string) error { return &ValidationError{Msg: msg} }

// NormalizeNotation accepts NdS+M, including the form where a query-string
// "+" was decoded as a space ("2d6 3" and "2d6+3" are the same roll).
func NormalizeNotation(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return "", invalid("notation is required")
	}
	if strings.ContainsAny(s, "+-") {
		s = strings.Join(strings.Fields(s), "")
	} else {
		s = strings.Join(strings.Fields(s), "+")
	}
	m := notationRE.FindStringSubmatch(s)
	if m == nil {
		return "", invalid("notation must look like 2d6+3, d20, or 4d8-2")
	}
	diceCount := 1
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return "", invalid("notation must look like 2d6+3, d20, or 4d8-2")
		}
		diceCount = n
	}
	sides, err := strconv.Atoi(m[2])
	if err != nil {
		return "", invalid("notation must look like 2d6+3, d20, or 4d8-2")
	}
	if diceCount < 1 || diceCount > 100 || sides < 2 || sides > 1000 {
		return "", invalid("notation allows 1-100 dice and 2-1000 sides")
	}
	if m[3] != "" {
		mod, err := strconv.Atoi(m[3])
		if err != nil || mod < -1000 || mod > 1000 {
			return "", invalid("modifier must be between -1000 and 1000")
		}
	}
	return s, nil
}

// NormalizeCount applies the public API default and limit.
func NormalizeCount(count int) (int, error) {
	if count == 0 {
		return 1, nil
	}
	if count < 1 || count > 100 {
		return 0, invalid("count must be from 1 to 100")
	}
	return count, nil
}
