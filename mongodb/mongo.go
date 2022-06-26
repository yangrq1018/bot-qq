package mongodb

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/net/proxy"
)

// NewClient creates a mongo DB client connecting the server on uri, optionally
// with a socks5 proxy.
func NewClient(uri, socks string) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri)
	if socks != "" {
		p, err := proxy.SOCKS5("tcp", socks, nil, proxy.Direct)
		if err != nil {
			return nil, err
		}
		opts = opts.SetDialer(proxy.NewPerHost(p, proxy.Direct))
	}
	client, err := mongo.Connect(context.TODO(), opts)
	return client, err
}
