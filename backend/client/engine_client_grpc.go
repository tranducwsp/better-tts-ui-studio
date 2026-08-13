package client

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "backend/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const maxMessageBytes = 50 * 1024 * 1024 // 50 MB

// GrpcTTSClient wraps a gRPC connection to the TTS engine.
// Only Synthesize uses gRPC; manifest/voices/clone stay HTTP.
type GrpcTTSClient struct {
	Target string
	Conn   *grpc.ClientConn
	Client pb.TTSServiceClient
}

// NewGrpcTTSClient dials the engine and returns a ready client, or an error.
func NewGrpcTTSClient(target string) (*GrpcTTSClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes)),
	)
	if err != nil {
		return nil, fmt.Errorf("gRPC connect failed %s: %w", target, err)
	}

	client := pb.NewTTSServiceClient(conn)
	log.Printf("gRPC TTS Client connected to %s", target)

	return &GrpcTTSClient{
		Target: target,
		Conn:   conn,
		Client: client,
	}, nil
}

// Close shuts down the gRPC connection.
func (c *GrpcTTSClient) Close() {
	if c.Conn != nil {
		_ = c.Conn.Close()
	}
}

// Synthesize calls the engine via gRPC. Signature matches CoreTTSClient.Synthesize.
func (c *GrpcTTSClient) Synthesize(ctx context.Context, text, voice string, speed float64, engine string, pitch *float64, emotion *string) ([]byte, error) {
	req := &pb.SynthesizeRequest{
		Text:    text,
		VoiceId: voice,
		Speed:   speed,
		Engine:  engine,
	}
	if pitch != nil {
		req.Pitch = pitch
	}
	if emotion != nil {
		req.Emotion = emotion
	}

	resp, err := c.Client.Synthesize(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC Synthesize: %w", err)
	}
	if resp.ErrorMsg != "" {
		return nil, fmt.Errorf("Core TTS Error (gRPC): %s", resp.ErrorMsg)
	}
	return resp.AudioBytes, nil
}
