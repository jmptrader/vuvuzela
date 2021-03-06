// Copyright 2016 David Lazar. All rights reserved.
// Use of this source code is governed by the GNU AGPL
// license that can be found in the LICENSE file.

// Package coordinator implements the entry/coordinator server.
package coordinator

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/net/context"

	"vuvuzela.io/alpenhorn/config"
	"vuvuzela.io/alpenhorn/errors"
	"vuvuzela.io/alpenhorn/typesocket"
	"vuvuzela.io/concurrency"
	"vuvuzela.io/vuvuzela/convo"
	"vuvuzela.io/vuvuzela/mixnet"
)

// Server is the coordinator (entry) server for the
// Vuvuzela conversation protocol.
type Server struct {
	Service    string
	PrivateKey ed25519.PrivateKey

	MixWait   time.Duration
	RoundWait time.Duration

	PersistPath             string
	ConfigServerPersistPath string

	mu             sync.Mutex
	round          uint32
	onions         []onion
	closed         bool
	shutdown       chan struct{}
	latestMixRound *MixRound

	hub          *typesocket.Hub
	configServer *config.Server

	mixnetClient *mixnet.Client
}

type onion struct {
	sender typesocket.Conn
	data   []byte
}

var ErrServerClosed = errors.New("coordinator: server closed")

func (srv *Server) Run() error {
	if srv.PersistPath == "" {
		return errors.New("no persist path specified")
	}

	mux := typesocket.NewMux(map[string]interface{}{
		"onion": srv.incomingOnion,
	})
	srv.hub = &typesocket.Hub{
		Mux: mux,
	}

	srv.mixnetClient = &mixnet.Client{
		Key: srv.PrivateKey,
	}

	srv.mu.Lock()
	srv.onions = make([]onion, 0, 128)
	srv.closed = false
	srv.shutdown = make(chan struct{})
	srv.mu.Unlock()

	go srv.loop()
	return nil
}

func (srv *Server) Close() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	// This could be better if we had Contexts everywhere,
	// but only tests should need to close the server.
	if !srv.closed {
		close(srv.shutdown)
		srv.closed = true
		return nil
	} else {
		return ErrServerClosed
	}
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/ws"):
		srv.hub.ServeHTTP(w, r)
	case strings.HasPrefix(r.URL.Path, "/config"):
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/config")
		srv.configServer.ServeHTTP(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

type OnionMsg struct {
	Round uint32
	Onion []byte
}

type NewRound struct {
	Round      uint32
	ConfigHash string
}

type MixRound struct {
	MixSettings   mixnet.RoundSettings
	MixSignatures [][]byte
	EndTime       time.Time
}

type RoundError struct {
	Round uint32
	Err   string
}

func (srv *Server) onConnect(c typesocket.Conn) error {
	srv.mu.Lock()
	mixRound := srv.latestMixRound
	srv.mu.Unlock()

	if mixRound != nil {
		err := c.Send("mix", mixRound)
		if err != nil {
			return err
		}
	}

	return nil
}

func (srv *Server) incomingOnion(c typesocket.Conn, o OnionMsg) {
	srv.mu.Lock()
	round := srv.round
	if o.Round == round {
		srv.onions = append(srv.onions, onion{
			sender: c,
			data:   o.Onion,
		})
	}
	srv.mu.Unlock()
	if o.Round != round {
		log.Errorf("got onion for wrong round (want %d, got %d)", round, o.Round)
		c.Send("error", RoundError{
			Round: o.Round,
			Err:   fmt.Sprintf("wrong round (want %d)", round),
		})
	}
}

func (srv *Server) loop() {
	for {
		currentConfig, configHash := srv.configServer.CurrentConfig()
		mixServers := currentConfig.Inner.(*convo.ConvoConfig).MixServers

		srv.mu.Lock()
		srv.round++
		round := srv.round

		logger := log.WithFields(log.Fields{"round": round})

		if err := srv.persistLocked(); err != nil {
			logger.Errorf("error persisting state: %s", err)
			srv.mu.Unlock()
			break
		}
		srv.mu.Unlock()

		logger.Info("Starting new round")

		srv.hub.Broadcast("newround", NewRound{
			Round:      round,
			ConfigHash: configHash,
		})

		time.Sleep(500 * time.Millisecond)

		// TODO perhaps pkg.NewRound, mixnet.NewRound, hub.Broadcast, etc
		// should take a Context for better cancelation.

		mixSettings := mixnet.RoundSettings{
			Service: "Convo",
			Round:   round,
		}
		mixSigs, err := srv.mixnetClient.NewRound(context.Background(), mixServers, &mixSettings)
		if err != nil {
			logger.WithFields(log.Fields{"call": "mixnet.NewRound"}).Error(err)
			if !srv.sleep(10 * time.Second) {
				break
			}
			continue
		}

		roundEnd := time.Now().Add(srv.MixWait)
		mixRound := &MixRound{
			MixSettings:   mixSettings,
			MixSignatures: mixSigs,
			EndTime:       roundEnd,
		}
		srv.mu.Lock()
		srv.latestMixRound = mixRound
		srv.mu.Unlock()

		logger.Info("Announcing mixnet settings")
		srv.hub.Broadcast("mix", mixRound)

		if !srv.sleep(srv.MixWait) {
			break
		}

		logger.Info("Running round")
		srv.mu.Lock()
		go srv.runRound(context.Background(), mixServers[0], round, srv.onions)
		srv.onions = make([]onion, 0, len(srv.onions))
		srv.mu.Unlock()

		logger.Info("Waiting for next round")
		if !srv.sleep(srv.RoundWait) {
			break
		}
	}

	log.Info("Shutting down")
}

func (srv *Server) sleep(d time.Duration) bool {
	timer := time.NewTimer(d)
	select {
	case <-srv.shutdown:
		timer.Stop()
		return false
	case <-timer.C:
		return true
	}
}

func (srv *Server) runRound(ctx context.Context, firstServer mixnet.PublicServerConfig, round uint32, out []onion) {
	onions := make([][]byte, len(out))
	senders := make([]typesocket.Conn, len(out))
	for i, o := range out {
		onions[i] = o.data
		senders[i] = o.sender
	}

	logger := log.WithFields(log.Fields{"round": round})
	logger.WithFields(log.Fields{"onions": len(onions)}).Info("start RunRound")
	start := time.Now()

	replies, err := srv.mixnetClient.RunRound(ctx, firstServer, srv.Service, round, onions)
	if err != nil {
		logger.WithFields(log.Fields{"call": "RunRound"}).Error(err)
		srv.hub.Broadcast("error", RoundError{Round: round, Err: "server error"})
		return
	}
	end := time.Now()
	logger.WithFields(log.Fields{"duration": end.Sub(start)}).Info("end RunRound")

	concurrency.ParallelFor(len(replies), func(p *concurrency.P) {
		for i, ok := p.Next(); ok; i, ok = p.Next() {
			senders[i].Send("reply", OnionMsg{
				Round: round,
				Onion: replies[i],
			})
		}
	})
}
