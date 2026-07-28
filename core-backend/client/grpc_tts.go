package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	pb "core-backend/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GrpcTTSClient quản lý kết nối gRPC high-performance tới Core-TTS Engine
type GrpcTTSClient struct {
	Target string
	Conn   *grpc.ClientConn
	Client pb.TTSServiceClient
}

// NewGrpcTTSClient khởi tạo kết nối gRPC với timeout kết nối an toàn
func NewGrpcTTSClient(target string) (*GrpcTTSClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối gRPC server %s: %w", target, err)
	}

	client := pb.NewTTSServiceClient(conn)
	log.Printf("Đã kết nối thành công gRPC TTS Client tới %s!", target)

	return &GrpcTTSClient{
		Target: target,
		Conn:   conn,
		Client: client,
	}, nil
}

// Close đóng kết nối gRPC sạch sẽ khi server tắt
func (c *GrpcTTSClient) Close() {
	if c.Conn != nil {
		_ = c.Conn.Close()
	}
}

// StreamStandardSynthesize thực hiện Server Streaming cho Standard Engine
func (c *GrpcTTSClient) StreamStandardSynthesize(ctx context.Context, text, voice string, speed float64, onChunk func(chunk *pb.TTSChunkResponse) error) error {
	stream, err := c.Client.SynthesizeStandard(ctx, &pb.StandardTTSRequest{
		Text:  text,
		Voice: voice,
		Speed: speed,
	})
	if err != nil {
		return err
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := onChunk(chunk); err != nil {
			return err
		}
	}

	return nil
}

// StreamCloneSynthesize thực hiện Server Streaming cho Clone Engine (Voice Cloning)
func (c *GrpcTTSClient) StreamCloneSynthesize(ctx context.Context, text, refAudioPath, refText string, speed float64, onChunk func(chunk *pb.TTSChunkResponse) error) error {
	stream, err := c.Client.SynthesizeClone(ctx, &pb.CloneTTSRequest{
		Text:         text,
		RefAudioPath: refAudioPath,
		RefText:      refText,
		Speed:        speed,
	})
	if err != nil {
		return err
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := onChunk(chunk); err != nil {
			return err
		}
	}

	return nil
}

// StreamFastSynthesize thực hiện Server Streaming cho Fast Engine
func (c *GrpcTTSClient) StreamFastSynthesize(ctx context.Context, text, voice string, speed float64, onChunk func(chunk *pb.TTSChunkResponse) error) error {
	stream, err := c.Client.SynthesizeFast(ctx, &pb.FastTTSRequest{
		Text:  text,
		Voice: voice,
		Speed: speed,
	})
	if err != nil {
		return err
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := onChunk(chunk); err != nil {
			return err
		}
	}

	return nil
}
