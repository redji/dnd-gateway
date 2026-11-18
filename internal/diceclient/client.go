package diceclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dnd-gateway/internal/dice"
	"dnd-gateway/internal/dicepb"
)

// Client calls the in-process or remote dice gRPC service.
type Client struct {
	api dicepb.DiceServiceClient
}

func New(api dicepb.DiceServiceClient) *Client {
	return &Client{api: api}
}

func (c *Client) Roll(ctx context.Context, req dice.Request) (dice.Result, error) {
	in := &dicepb.RollRequest{
		Notation: req.Notation,
		Count:    int32(req.Count),
		Seed:     req.Seed,
	}
	resp, err := c.api.Roll(ctx, in)
	if err != nil {
		return dice.Result{}, fromStatus(err)
	}
	out := dice.Result{
		Rolls:    make([]dice.Roll, 0, len(resp.GetRolls())),
		Seed:     resp.GetSeed(),
		Count:    int(resp.GetCount()),
		Notation: resp.GetNotation(),
	}
	for _, roll := range resp.GetRolls() {
		faces := make([]int, len(roll.GetDice()))
		for i, face := range roll.GetDice() {
			faces[i] = int(face)
		}
		out.Rolls = append(out.Rolls, dice.Roll{
			Index:    int(roll.GetIndex()),
			Notation: roll.GetNotation(),
			Dice:     faces,
			Modifier: int(roll.GetModifier()),
			Total:    int(roll.GetTotal()),
		})
	}
	return out, nil
}

// CallError carries the HTTP status the gateway should return for an RPC failure.
type CallError struct {
	Status int
	Msg    string
}

func (e *CallError) Error() string { return e.Msg }

func fromStatus(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return &CallError{Status: 502, Msg: "dice service is unavailable"}
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return &CallError{Status: 400, Msg: st.Message()}
	case codes.NotFound:
		return &CallError{Status: 404, Msg: st.Message()}
	case codes.DeadlineExceeded:
		return &CallError{Status: 504, Msg: "dice service timed out"}
	default:
		return &CallError{Status: 502, Msg: fmt.Sprintf("dice service: %s", st.Message())}
	}
}
