package diceserver

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dnd-gateway/internal/dice"
	"dnd-gateway/internal/dicepb"
	"dnd-gateway/internal/diceupstream"
)

// Server is the gRPC wrapper around the public dice API.
type Server struct {
	dicepb.UnimplementedDiceServiceServer
	upstream *diceupstream.Client
}

func New(upstream *diceupstream.Client) *Server {
	return &Server{upstream: upstream}
}

func (s *Server) Roll(ctx context.Context, req *dicepb.RollRequest) (*dicepb.RollResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	result, err := s.upstream.Roll(ctx, dice.Request{
		Notation: req.GetNotation(),
		Count:    int(req.GetCount()),
		Seed:     req.Seed,
	})
	if err != nil {
		return nil, toStatus(err)
	}

	out := &dicepb.RollResponse{
		Seed:     result.Seed,
		Count:    int32(result.Count),
		Notation: result.Notation,
		Rolls:    make([]*dicepb.DieRoll, 0, len(result.Rolls)),
	}
	for _, roll := range result.Rolls {
		faces := make([]int32, len(roll.Dice))
		for i, face := range roll.Dice {
			faces[i] = int32(face)
		}
		out.Rolls = append(out.Rolls, &dicepb.DieRoll{
			Index:    int32(roll.Index),
			Notation: roll.Notation,
			Dice:     faces,
			Modifier: int32(roll.Modifier),
			Total:    int32(roll.Total),
		})
	}
	return out, nil
}

func toStatus(err error) error {
	var ve *dice.ValidationError
	if errors.As(err, &ve) {
		return status.Error(codes.InvalidArgument, ve.Error())
	}
	var se *diceupstream.StatusError
	if errors.As(err, &se) {
		switch se.Status {
		case 400:
			return status.Error(codes.InvalidArgument, "dice upstream rejected the roll")
		case 404:
			return status.Error(codes.NotFound, "dice upstream route was not found")
		default:
			return status.Error(codes.Unavailable, "dice upstream is unavailable")
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "dice upstream timed out")
	}
	return status.Error(codes.Unavailable, "dice upstream is unavailable")
}
