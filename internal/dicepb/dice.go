// Package dicepb is the internal dice RPC API matching proto/dice/v1/dice.proto.
// Messages travel over gRPC with a JSON codec so the service builds without
// running protoc on every machine. The .proto file remains the contract.
package dicepb

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const serviceName = "dice.v1.DiceService"

// RollRequest is one roll batch.
type RollRequest struct {
	Notation string `json:"notation"`
	Count    int32  `json:"count"`
	Seed     *int64 `json:"seed,omitempty"`
}

func (x *RollRequest) GetNotation() string {
	if x == nil {
		return ""
	}
	return x.Notation
}

func (x *RollRequest) GetCount() int32 {
	if x == nil {
		return 0
	}
	return x.Count
}

// DieRoll is one independent roll of the notation.
type DieRoll struct {
	Index    int32   `json:"index"`
	Notation string  `json:"notation"`
	Dice     []int32 `json:"dice"`
	Modifier int32   `json:"modifier"`
	Total    int32   `json:"total"`
}

func (x *DieRoll) GetIndex() int32 {
	if x == nil {
		return 0
	}
	return x.Index
}

func (x *DieRoll) GetNotation() string {
	if x == nil {
		return ""
	}
	return x.Notation
}

func (x *DieRoll) GetDice() []int32 {
	if x == nil {
		return nil
	}
	return x.Dice
}

func (x *DieRoll) GetModifier() int32 {
	if x == nil {
		return 0
	}
	return x.Modifier
}

func (x *DieRoll) GetTotal() int32 {
	if x == nil {
		return 0
	}
	return x.Total
}

// RollResponse is the dice service reply.
type RollResponse struct {
	Rolls    []*DieRoll `json:"rolls"`
	Seed     int64      `json:"seed"`
	Count    int32      `json:"count"`
	Notation string     `json:"notation"`
}

func (x *RollResponse) GetRolls() []*DieRoll {
	if x == nil {
		return nil
	}
	return x.Rolls
}

func (x *RollResponse) GetSeed() int64 {
	if x == nil {
		return 0
	}
	return x.Seed
}

func (x *RollResponse) GetCount() int32 {
	if x == nil {
		return 0
	}
	return x.Count
}

func (x *RollResponse) GetNotation() string {
	if x == nil {
		return ""
	}
	return x.Notation
}

// DiceServiceServer is implemented by the dice microservice.
type DiceServiceServer interface {
	Roll(context.Context, *RollRequest) (*RollResponse, error)
	mustEmbedUnimplementedDiceServiceServer()
}

// UnimplementedDiceServiceServer must be embedded for forward compatibility.
type UnimplementedDiceServiceServer struct{}

func (UnimplementedDiceServiceServer) Roll(context.Context, *RollRequest) (*RollResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Roll not implemented")
}

func (UnimplementedDiceServiceServer) mustEmbedUnimplementedDiceServiceServer() {}

// UnsafeDiceServiceServer may break forward compatibility.
type UnsafeDiceServiceServer interface {
	mustEmbedUnimplementedDiceServiceServer()
}

// RegisterDiceServiceServer registers the service on a gRPC server.
func RegisterDiceServiceServer(s grpc.ServiceRegistrar, srv DiceServiceServer) {
	s.RegisterService(&DiceService_ServiceDesc, srv)
}

func _DiceService_Roll_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(RollRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DiceServiceServer).Roll(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dice.v1.DiceService/Roll",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(DiceServiceServer).Roll(ctx, req.(*RollRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// DiceService_ServiceDesc is the gRPC service descriptor.
var DiceService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: serviceName,
	HandlerType: (*DiceServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Roll",
			Handler:    _DiceService_Roll_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/dice/v1/dice.proto",
}

// DiceServiceClient is the client API.
type DiceServiceClient interface {
	Roll(ctx context.Context, in *RollRequest, opts ...grpc.CallOption) (*RollResponse, error)
}

type diceServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewDiceServiceClient returns a client for DiceService.
func NewDiceServiceClient(cc grpc.ClientConnInterface) DiceServiceClient {
	return &diceServiceClient{cc}
}

func (c *diceServiceClient) Roll(ctx context.Context, in *RollRequest, opts ...grpc.CallOption) (*RollResponse, error) {
	out := new(RollResponse)
	err := c.cc.Invoke(ctx, "/dice.v1.DiceService/Roll", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Codec is a JSON payload codec for this service's messages.
type Codec struct{}

func (Codec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (Codec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (Codec) Name() string {
	return "json"
}

// ServerOptions forces the JSON codec on a gRPC server.
func ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.ForceServerCodec(Codec{})}
}

// DialOptions forces the JSON codec on a gRPC client.
func DialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.ForceCodec(Codec{})),
	}
}
