// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fileserver

import (
	"encoding/json"
	"flag"
	"log"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	caddytpl "github.com/caddyserver/caddy/v2/modules/caddyhttp/templates"
	"github.com/caddyserver/certmagic"
	"go.uber.org/zap"
)

func init() {
	caddycmd.RegisterCommand(caddycmd.Command{
		Name:  "file-server",
		Func:  cmdFileServer,
		Usage: "[--domain <example.com>] [--root <path>] [--listen <addr>] [--browse] [--access-log]",
		Short: "Spins up a production-ready file server",
		Long: `
A simple but production-ready file server. Useful for quick deployments,
demos, and development.

The listener's socket address can be customized with the --listen flag.

If a domain name is specified with --domain, the default listener address
will be changed to the HTTPS port and the server will use HTTPS. If using
a public domain, ensure A/AAAA records are properly configured before
using this option.

If --browse is enabled, requests for folders without an index file will
respond with a file listing.`,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("file-server", flag.ExitOnError)
			fs.String("domain", "", "Domain name at which to serve the files")
			fs.String("root", "", "The path to the root of the site")
			fs.Int("count", 20000, "The limit of how many files to autoindex")
			fs.String("listen", "", "The address to which to bind the listener")
			fs.Bool("browse", false, "Enable directory browsing")
			fs.Bool("templates", false, "Enable template rendering")
			fs.Bool("access-log", false, "Enable the access log")
			fs.Bool("debug", false, "Enable verbose debug logs")
			return fs
		}(),
	})
}

func cmdFileServer(fs caddycmd.Flags) (int, error) {
	caddy.TrapSignals()

	domain := fs.String("domain")
	root := fs.String("root")
	count := fs.Int("count")
	listen := fs.String("listen")
	browse := fs.Bool("browse")
	templates := fs.Bool("templates")
	accessLog := fs.Bool("access-log")
	debug := fs.Bool("debug")

	var handlers []json.RawMessage

	if templates {
		handler := caddytpl.Templates{FileRoot: root}
		handlers = append(handlers, caddyconfig.JSONModuleObject(handler, "handler", "templates", nil))
	}
	handler := FileServer{Root: root, Count: count}

	if browse {
		handler.Browse = new(Browse)
	}
	handlers = append(handlers, caddyconfig.JSONModuleObject(handler, "handler", "file_server", nil))

	route := caddyhttp.Route{HandlersRaw: handlers}

	if domain != "" {
		route.MatcherSetsRaw = []caddy.ModuleMap{
			{
				"host": caddyconfig.JSON(caddyhttp.MatchHost{domain}, nil),
			},
		}
	}

	server := &caddyhttp.Server{
		ReadHeaderTimeout: caddy.Duration(10 * time.Second),
		IdleTimeout:       caddy.Duration(30 * time.Second),
		MaxHeaderBytes:    1024 * 10,
		Routes:            caddyhttp.RouteList{route},
	}
	if listen == "" {
		if domain == "" {
			listen = ":80"
		} else {
			listen = ":" + strconv.Itoa(certmagic.HTTPSPort)
		}
	}
	server.Listen = []string{listen}
	if accessLog {
		server.Logs = &caddyhttp.ServerLogConfig{}
	}

	httpApp := caddyhttp.App{
		Servers: map[string]*caddyhttp.Server{"static": server},
	}

	var false bool
	cfg := &caddy.Config{
		Admin: &caddy.AdminConfig{
			Disabled: true,
			Config: &caddy.ConfigSettings{
				Persist: &false,
			},
		},
		AppsRaw: caddy.ModuleMap{
			"http": caddyconfig.JSON(httpApp, nil),
		},
	}

	if debug {
		cfg.Logging = &caddy.Logging{
			Logs: map[string]*caddy.CustomLog{
				"default": {Level: zap.DebugLevel.CapitalString()},
			},
		}
	}

	err := caddy.Run(cfg)
	if err != nil {
		return caddy.ExitCodeFailedStartup, err
	}

	log.Printf("Caddy serving static files on %s", listen)

	select {}
}
