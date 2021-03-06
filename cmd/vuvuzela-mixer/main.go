// Copyright 2016 David Lazar. All rights reserved.
// Use of this source code is governed by the GNU AGPL
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"text/template"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"vuvuzela.io/alpenhorn/edtls"
	"vuvuzela.io/alpenhorn/encoding/toml"
	"vuvuzela.io/crypto/rand"
	"vuvuzela.io/vuvuzela/convo"
	"vuvuzela.io/vuvuzela/mixnet"
	pb "vuvuzela.io/vuvuzela/mixnet/convopb"
)

var (
	confPath = flag.String("conf", "", "config file")
	doinit   = flag.Bool("init", false, "create config file")
)

type Config struct {
	ListenAddr string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey

	CoordinatorKey ed25519.PublicKey

	Noise rand.Laplace
}

var funcMap = template.FuncMap{
	"base32": toml.EncodeBytes,
}

const confTemplate = `# Vuvuzela mixnet server config

listenAddr = {{.ListenAddr | printf "%q"}}

publicKey  = {{.PublicKey | base32 | printf "%q"}}
privateKey = {{.PrivateKey | base32 | printf "%q"}}

coordinatorKey = "change me"

[noise]
mu = {{.Noise.Mu | printf "%0.1f"}}
b = {{.Noise.B | printf "%0.1f"}}
`

func writeNewConfig() {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	conf := &Config{
		ListenAddr: "0.0.0.0:2718",
		PublicKey:  publicKey,
		PrivateKey: privateKey,

		Noise: rand.Laplace{
			Mu: 100,
			B:  3.0,
		},
	}

	tmpl := template.Must(template.New("config").Funcs(funcMap).Parse(confTemplate))

	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, conf)
	if err != nil {
		log.Fatalf("template error: %s", err)
	}
	data := buf.Bytes()

	path := "vuvuzela-mixer-init.conf"
	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s\n", path)
}

func main() {
	flag.Parse()

	if *doinit {
		writeNewConfig()
		return
	}

	if *confPath == "" {
		fmt.Println("specify config file with -conf")
		os.Exit(1)
	}

	data, err := ioutil.ReadFile(*confPath)
	if err != nil {
		log.Fatal(err)
	}
	conf := new(Config)
	err = toml.Unmarshal(data, conf)
	if err != nil {
		log.Fatalf("error parsing config %q: %s", *confPath, err)
	}

	mixServer := &mixnet.Server{
		SigningKey:     conf.PrivateKey,
		CoordinatorKey: conf.CoordinatorKey,

		Services: map[string]mixnet.MixService{
			"Convo": &convo.ConvoService{
				Laplace:      conf.Noise,
				AccessCounts: make(chan convo.AccessCount, 64),
			},
		},
	}

	/*
		// Histogram is disabled for now since the chain is dynamic:
		// we might stop being the last server at any moment.
		if lastServer {
			histogram := &Histogram{Mu: conf.Noise.Mu, NumServers: len(mixers)}
			go histogram.run(mixServer.AccessCounts)
		}
	*/

	creds := credentials.NewTLS(edtls.NewTLSServerConfig(conf.PrivateKey))
	grpcServer := grpc.NewServer(grpc.Creds(creds))

	pb.RegisterMixnetServer(grpcServer, mixServer)

	log.Infof("Listening on %q", conf.ListenAddr)
	listener, err := net.Listen("tcp", conf.ListenAddr)
	if err != nil {
		log.Fatalf("net.Listen: %s", err)
	}

	err = grpcServer.Serve(listener)
	log.Fatal(err)
}
