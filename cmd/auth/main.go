// Copyright 2019 Tad Lebeck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-openapi/runtime/flagext"
	gtx "github.com/gorilla/context"
	"github.com/jessevdk/go-flags"
	"github.com/julienschmidt/httprouter"
	"github.com/op/go-logging"
)

// LoginParams is the object required for login
type LoginParams struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Auth is the response object for login and validation requests
type Auth struct {
	Token    string `json:"token"`
	Expiry   int64  `json:"exp"`
	Issuer   string `json:"iss"`
	Username string `json:"username"`
}

// AuthClaims is the claims specified jwt
type AuthClaims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

const (
	// Issuer is the issuer (iss) for the jwt standard claim
	Issuer = "Nuvoloso"
	// SessionName is the name of session the store will retrieve, using Gorilla Sessions
	SessionName = "nv-auth-session"
)

// Build information passed in via ldflags
var (
	BuildID   string
	BuildTime string
	BuildHost string
	BuildJob  string
	Appname   string
)

// Ini file name template (a var for testing purposes)
var iniFileNameTemplate = "/etc/nuvoloso/%s.ini"

// ServerArgs are the creation arguments for the application, based loosely on the REST API arguments used by other services
type ServerArgs struct {
	EnabledListener string           `long:"scheme" description:"the listener to enable" default:"http" choice:"http" choice:"https"`
	CleanupTimeout  time.Duration    `long:"cleanup-timeout" description:"grace period for which to wait before shutting down the server" default:"10s"`
	MaxHeaderSize   flagext.ByteSize `long:"max-header-size" description:"controls the maximum number of bytes the server will read parsing the request header's keys and values, including the request line. It does not limit the size of the request body" default:"1MiB"`

	Host         string        `long:"host" description:"the IP to listen on" default:"localhost" env:"HOST"`
	Port         int           `long:"port" description:"the port to listen on for insecure connections" default:"5555" env:"PORT"`
	ReadTimeout  time.Duration `long:"read-timeout" description:"maximum duration before timing out read of the request" default:"30s"`
	WriteTimeout time.Duration `long:"write-timeout" description:"maximum duration before timing out write of the response" default:"60s"`

	TLSHost           string         `long:"tls-host" description:"the IP to listen on for tls, when not specified it's the same as --host" env:"TLS_HOST"`
	TLSPort           int            `long:"tls-port" description:"the port to listen on for secure connections" default:"5555" env:"TLS_PORT"`
	TLSCertificate    flags.Filename `long:"tls-certificate" description:"the certificate to use for secure connections" env:"TLS_CERTIFICATE"`
	TLSCertificateKey flags.Filename `long:"tls-key" description:"the private key to use for secure connections" env:"TLS_PRIVATE_KEY"`
	TLSCACertificate  flags.Filename `long:"tls-ca" description:"the certificate authority file to be used with mutual tls auth" env:"TLS_CA_CERTIFICATE"`
	TLSReadTimeout    time.Duration  `long:"tls-read-timeout" description:"maximum duration before timing out read of the request"`
	TLSWriteTimeout   time.Duration  `long:"tls-write-timeout" description:"maximum duration before timing out write of the response"`
}

// mainContext contains context information for this package
type mainContext struct {
	MongoArgs       mongodb.Args `group:"Mongo Options" namespace:"mongo"`
	ServerArgs      *ServerArgs  `no-flag:"yes"`
	ServiceVersion  string
	server          *http.Server
	idleConnsClosed chan struct{}
	sigChan         chan os.Signal
	invocationArgs  string
	db              mongodb.DBAPI
	log             *logging.Logger
	store           *cookieStore
	*auth.Extractor

	Expiry   time.Duration  `long:"expiry" description:"The time interval after which tokens expire" default:"2h"`
	LogLevel string         `long:"log-level" description:"Specify the minimum logging level" default:"DEBUG" choice:"DEBUG" choice:"INFO" choice:"WARNING" choice:"ERROR"`
	Version  bool           `long:"version" short:"V" description:"Shows the build version and time created" no-ini:"1" json:"-"`
	WriteIni flags.Filename `long:"write-config" description:"Specify the name of a file to which to write the current configuration" no-ini:"1" json:"-"`
}

var errNotReady = errors.New("auth service is not ready")

func (app *mainContext) getNewAuth(username string, expiry int64) []byte {
	if expiry == 0 {
		expiry = app.getNewExpiry()
	}
	claims := AuthClaims{
		username,
		jwt.StandardClaims{
			ExpiresAt: expiry,
			Issuer:    Issuer,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, _ := token.SignedString(app.store.Secret())

	auth := Auth{
		Token:    tokenString,
		Expiry:   expiry,
		Issuer:   Issuer,
		Username: username,
	}
	marshalled, _ := json.Marshal(auth) // this Marshal never fails
	return marshalled
}

func (app *mainContext) getNewExpiry() int64 {
	return time.Now().Add(app.Expiry).Unix()
}

func (app *mainContext) login(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	dec := json.NewDecoder(r.Body)

	params := LoginParams{}
	err := dec.Decode(&params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	store, err := app.store.Store(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dbUser, err := findUser(r.Context(), app.db, params)
	if err != nil {
		if err == errInvalidUser || err == errDisabledUser {
			http.Error(w, err.Error(), http.StatusUnauthorized)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	ret := app.getNewAuth(dbUser.AuthIdentifier, 0)

	session, _ := store.Get(r, SessionName)
	session.Values["authenticated"] = true
	session.Save(r, w)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(ret)
}

func (app *mainContext) logout(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	store, err := app.store.Store(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session, _ := store.Get(r, SessionName)
	session.Values["authenticated"] = false
	session.Save(r, w)
}

func (app *mainContext) validate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	store, err := app.store.Store(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session, _ := store.Get(r, SessionName)
	// a new session would not have been "logged out", so skip check for session's authenticated flag
	if !session.IsNew {
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	tokenString := r.Header.Get("token")

	token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(token *jwt.Token) (interface{}, error) {
		return app.store.Secret(), nil
	})

	if token == nil {
		http.Error(w, "Unable to parse token", http.StatusUnauthorized)
		return
	}

	if !token.Valid && err != nil {
		ve, ok := err.(*jwt.ValidationError)
		if ok {
			if ve.Errors&jwt.ValidationErrorExpired != 0 {
				http.Error(w, "Expired token", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || len(claims.Username) < 1 {
		http.Error(w, "Invalid JWT claims", http.StatusUnauthorized)
		return
	}
	username := claims.Username
	expiry := claims.ExpiresAt
	if r.URL.Query().Get("preserve-expiry") != "true" {
		expiry = 0
	}
	ret := app.getNewAuth(username, expiry)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(ret)
}

// Register handles all preparation for the process to be served
func (app *mainContext) Register() http.Handler {
	router := httprouter.New()
	router.POST("/auth/login", app.login)
	router.POST("/auth/logout", app.logout)
	router.POST("/auth/validate", app.validate)

	return router
}

type dbAPIFn func(args *mongodb.Args) mongodb.DBAPI
type exitFn func(int)
type shutdownFn func(context.Context, *http.Server) error

var dbAPIHook dbAPIFn = mongodb.NewDBAPI
var exitHook exitFn = os.Exit
var shutdownHook shutdownFn = func(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}

// parseArgs will parse the INI file and command line args.
// It calls os.Exit(0) if --help, --version or --write-config are specified
func (app *mainContext) parseArgs() error {
	iniFile := fmt.Sprintf(iniFileNameTemplate, Appname)
	parser := flags.NewParser(app, flags.Default)
	parser.ShortDescription = Appname
	parser.LongDescription = "The Nuvoloso authentication service.\n\n" +
		"The service reads its configuration options from " + iniFile + " on startup. " +
		"Command line flags will override the file values."

	parser.AddGroup("Service Options", "", app.ServerArgs)
	if e := flags.NewIniParser(parser).ParseFile(iniFile); e != nil {
		if !os.IsNotExist(e) {
			return e
		}
		app.log.Info("Configuration file", iniFile, "not found")
	} else {
		app.log.Info("Read configuration defaults from", iniFile)
	}

	if _, err := parser.Parse(); err != nil {
		code := 1
		if fe, ok := err.(*flags.Error); ok {
			if fe.Type == flags.ErrHelp {
				code = 0
			}
		}
		exitHook(code)
	}
	if app.Version {
		fmt.Println("Build ID:", BuildID)
		fmt.Println("Build date:", BuildTime)
		fmt.Println("Build host:", BuildHost)
		if BuildJob != "" {
			fmt.Println("Build job:", BuildJob)
		}
		exitHook(0)
	}
	if app.WriteIni != "" {
		iniP := flags.NewIniParser(parser)
		iniP.WriteFile(string(app.WriteIni), flags.IniIncludeComments|flags.IniIncludeDefaults|flags.IniCommentDefaults)
		fmt.Println("Wrote configuration to", app.WriteIni)
		exitHook(0)
	}
	return nil
}

// setupLogging initializes logging for the application
func (app *mainContext) setupLogging() {
	logLevel, err := logging.LogLevel(app.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %s", app.LogLevel)
		exitHook(1)
	}
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	formatter := logging.MustStringFormatter("%{time} %{level:.1s} %{shortfile} %{message}")
	formatted := logging.NewBackendFormatter(backend, formatter)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logLevel, "")
	logging.SetBackend(leveled)

	app.log = logging.MustGetLogger("")
}

func (app *mainContext) init() error {
	app.Extractor = &auth.Extractor{}
	server := &http.Server{
		Handler:        gtx.ClearHandler(app.Register()),
		MaxHeaderBytes: int(app.ServerArgs.MaxHeaderSize),
	}
	if app.ServerArgs.EnabledListener == "http" {
		server.Addr = fmt.Sprintf("%s:%d", app.ServerArgs.Host, app.ServerArgs.Port)
		server.ReadTimeout = app.ServerArgs.ReadTimeout
		server.ReadHeaderTimeout = app.ServerArgs.ReadTimeout
		server.WriteTimeout = app.ServerArgs.WriteTimeout
	} else {
		// https
		host := app.ServerArgs.TLSHost
		if host == "" {
			host = app.ServerArgs.Host
		}
		server.Addr = fmt.Sprintf("%s:%d", host, app.ServerArgs.TLSPort)
		server.ReadTimeout = app.ServerArgs.TLSReadTimeout
		server.ReadHeaderTimeout = app.ServerArgs.TLSReadTimeout
		server.WriteTimeout = app.ServerArgs.TLSWriteTimeout
		var err error
		if server.TLSConfig, err = app.tlsConfig(); err != nil {
			return err
		}
		server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0) // magic! force HTTP/1.1
	}

	app.MongoArgs.Log = app.log
	app.MongoArgs.AppName = Appname
	app.MongoArgs.TLSCertificate = string(app.ServerArgs.TLSCertificate)
	app.MongoArgs.TLSCertificateKey = string(app.ServerArgs.TLSCertificateKey)
	app.MongoArgs.TLSCACertificate = string(app.ServerArgs.TLSCACertificate)
	app.db = dbAPIHook(&app.MongoArgs)
	if err := app.db.Connect(nil); err != nil {
		return err
	}
	app.store = newCookieStore(app.db)

	app.idleConnsClosed = make(chan struct{})
	go func() {
		app.sigChan = make(chan os.Signal, 1)
		signal.Notify(app.sigChan, os.Interrupt, os.Signal(syscall.SIGTERM))
		<-app.sigChan
		app.log.Infof("%s server shutting down", app.ServerArgs.EnabledListener)

		app.db.Terminate()
		ctx, cancel := context.WithTimeout(context.Background(), app.ServerArgs.CleanupTimeout)
		defer cancel()
		if err := shutdownHook(ctx, server); err != nil {
			// Error from closing listeners, or context timeout
			app.log.Errorf("%s server shutdown: %v", app.ServerArgs.EnabledListener, err)
		} else {
			app.log.Infof("%s server shutdown succeeded", app.ServerArgs.EnabledListener)
		}
		close(app.idleConnsClosed)
	}()

	app.server = server
	return nil
}

// Set up TLS configuration, including the given CA certificate.
func (app *mainContext) tlsConfig() (*tls.Config, error) {
	if app.ServerArgs.TLSCACertificate == "" || app.ServerArgs.TLSCertificate == "" || app.ServerArgs.TLSCertificateKey == "" {
		return nil, errors.New("tls-ca, tls-certificate and tls-key are required with scheme=https")
	}
	// load CA crt
	caCert, err := ioutil.ReadFile(string(app.ServerArgs.TLSCACertificate))
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("no CA certs found in %s", app.ServerArgs.TLSCACertificate)
	}

	// Based on https://blog.bracebin.com/achieving-perfect-ssl-labs-score-with-go
	cfg := &tls.Config{
		ClientCAs:                caCertPool,
		ClientAuth:               tls.VerifyClientCertIfGiven,
		NextProtos:               []string{"http/1.1"},
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}
	cfg.BuildNameToCertificate()
	return cfg, nil
}

func (app *mainContext) listenAndServe() error {
	var err error
	if app.ServerArgs.EnabledListener == "http" {
		err = app.server.ListenAndServe()
	} else {
		err = app.server.ListenAndServeTLS(string(app.ServerArgs.TLSCertificate), string(app.ServerArgs.TLSCertificateKey))
	}
	if err == http.ErrServerClosed {
		err = nil
		<-app.idleConnsClosed
	}
	return err
}

func (app *mainContext) logStart() {
	buildContext := BuildJob
	if buildContext == "" {
		buildContext = BuildHost
	}
	app.ServiceVersion = BuildID + " " + BuildTime + " " + buildContext
	if b, err := json.MarshalIndent(app, "", "    "); err == nil {
		// TBD also log periodically if additional logging is added
		app.invocationArgs = string(b)
		app.log.Info(app.invocationArgs)
	}
	app.log.Infof("[%s] running on %s:%s", Appname, app.ServerArgs.EnabledListener, app.server.Addr)
}

func main() {
	app := &mainContext{ServerArgs: &ServerArgs{}}
	app.LogLevel = "DEBUG" // for bootstrap
	app.setupLogging()
	err := app.parseArgs()
	if err == nil {
		app.setupLogging() // again
		err = app.init()
	}
	if err == nil {
		app.logStart()
		err = app.listenAndServe()
	}
	if err != nil {
		app.log.Critical(err)
		exitHook(1)
	}
}
