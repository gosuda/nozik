package gzk

import (
	"context"
	"fmt"
	"time"

	musicv1 "github.com/gg582/gozik/api/music/v1"
	playerv1 "github.com/gg582/gozik/api/player/v1"
	"github.com/gg582/nozik/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a gRPC client for Gozik.
type Client struct {
	conn   *grpc.ClientConn
	player playerv1.PlayerServiceClient
}

// Dial connects to Gozik at addr.
func Dial(addr string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("dial gozik %s: %w", addr, err)
	}
	return &Client{conn: conn, player: playerv1.NewPlayerServiceClient(conn)}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error { return c.conn.Close() }

// SearchTracks calls Gozik's provider search.
func (c *Client) SearchTracks(ctx context.Context, providerID, query string, limit int32) ([]*musicv1.Track, error) {
	resp, err := c.player.SearchTracks(ctx, &playerv1.SearchTracksRequest{
		ProviderId: providerID,
		Query:      query,
		Limit:      limit,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetTracks(), nil
}

// AddProviderTrack queues a provider track in Gozik.
func (c *Client) AddProviderTrack(ctx context.Context, providerID, trackID string) error {
	_, err := c.player.AddProviderTrack(ctx, &playerv1.AddProviderTrackRequest{
		ProviderId: providerID,
		TrackId:    trackID,
	})
	return err
}

// AddToQueue adds local file paths to Gozik's queue.
func (c *Client) AddToQueue(ctx context.Context, paths []string) error {
	_, err := c.player.AddToQueue(ctx, &playerv1.QueueRequest{Paths: paths})
	return err
}

// Play starts playback.
func (c *Client) Play(ctx context.Context) error {
	_, err := c.player.Play(ctx, &playerv1.Empty{})
	return err
}

// GetStatus returns Gozik's current status.
func (c *Client) GetStatus(ctx context.Context) (*playerv1.Status, error) {
	return c.player.GetStatus(ctx, &playerv1.Empty{})
}

// ListProviders returns available music providers.
func (c *Client) ListProviders(ctx context.Context) ([]*playerv1.ProviderInfo, error) {
	resp, err := c.player.ListProviders(ctx, &playerv1.ListProvidersRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetProviders(), nil
}

// QueueItemToRecommendation converts a music.v1.Track to a model.Recommendation.
func TrackToRecommendation(t *musicv1.Track) model.Recommendation {
	artist := ""
	if len(t.GetArtists()) > 0 {
		artist = t.GetArtists()[0].GetName()
	}
	album := ""
	if t.GetAlbum() != nil {
		album = t.GetAlbum().GetTitle()
	}
	return model.Recommendation{
		Title:           t.GetTitle(),
		Artist:          artist,
		Album:           album,
		ProviderID:      "yt-music", // default when resolving via Gozik provider search
		ProviderTrackID: t.GetId(),
	}
}
