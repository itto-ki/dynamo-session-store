// Copyright (c) 2022 Ittoh Kimura
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"bytes"
	"context"
	"encoding/gob"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gorilla/sessions"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
)

const tableName = "test_table"

// NewRecorder returns an initialized ResponseRecorder.
func NewRecorder() *httptest.ResponseRecorder {
	return &httptest.ResponseRecorder{
		Body: new(bytes.Buffer),
	}
}

// ----------------------------------------------------------------------------

type FlashMessage struct {
	Type    int
	Message string
}

func TestMain(m *testing.M) {
	setup()
	ret := m.Run()
	teardown()
	os.Exit(ret)
}

func setup() {
	os.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	cfg, err := loadConfig()
	if err != nil {
		panic(err.Error())
	}
	dynamoClient := dynamodb.NewFromConfig(cfg)
	_, err = dynamoClient.CreateTable(context.TODO(), &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("id"),
				KeyType:       "HASH",
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("id"),
				AttributeType: "S",
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})
	if err != nil {
		panic(err.Error())
	}
}

func teardown() {
	cfg, err := loadConfig()
	if err != nil {
		panic(err.Error())
	}
	dynamoClient := dynamodb.NewFromConfig(cfg)
	_, err = dynamoClient.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	if err != nil {
		panic(err.Error())
	}
}

// loadConfig load an AWS configuration for DynamoDB-local
func loadConfig() (aws.Config, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{URL: "http://localhost:8000"}, nil
	})
	return config.LoadDefaultConfig(context.TODO(), config.WithEndpointResolverWithOptions(customResolver))
}

func TestFlashes(t *testing.T) {
	var req *http.Request
	var rsp *httptest.ResponseRecorder
	var hdr http.Header
	var err error
	var ok bool
	var cookies []string
	var session *sessions.Session
	var flashes []interface{}

	cfg, err := loadConfig()
	if err != nil {
		panic(err.Error())
	}
	store := NewStore(tableName, cfg, []byte("secret-key"))

	// Round 1 ----------------------------------------------------------------

	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)
	rsp = NewRecorder()
	// Get a session.
	if session, err = store.Get(req, "session-key"); err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	// Get a flash.
	flashes = session.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected empty flashes; Got %v", flashes)
	}
	// Add some flashes.
	session.AddFlash("foo")
	session.AddFlash("bar")
	// Custom key.
	session.AddFlash("baz", "custom_key")
	// Save.
	if err = store.Save(req, rsp, session); err != nil {
		t.Fatalf("Error saving session: %v", err)
	}
	hdr = rsp.Header()
	cookies, ok = hdr["Set-Cookie"]
	if !ok || len(cookies) != 1 {
		t.Fatal("No cookies. Header:", hdr)
	}

	if _, err = store.Get(req, "session:key"); err.Error() != "sessions: invalid character in cookie name: session:key" {
		t.Fatalf("Expected error due to invalid cookie name")
	}

	// Round 2 ----------------------------------------------------------------

	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)
	req.Header.Add("Cookie", cookies[0])
	rsp = NewRecorder() // nolint
	// Get a session.
	if session, err = store.Get(req, "session-key"); err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	// Check all saved values.
	flashes = session.Flashes()
	if len(flashes) != 2 {
		t.Fatalf("Expected flashes; Got %v", flashes)
	}
	if flashes[0] != "foo" || flashes[1] != "bar" {
		t.Errorf("Expected foo,bar; Got %v", flashes)
	}
	flashes = session.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected dumped flashes; Got %v", flashes)
	}
	// Custom key.
	flashes = session.Flashes("custom_key")
	if len(flashes) != 1 {
		t.Errorf("Expected flashes; Got %v", flashes)
	} else if flashes[0] != "baz" {
		t.Errorf("Expected baz; Got %v", flashes)
	}
	flashes = session.Flashes("custom_key")
	if len(flashes) != 0 {
		t.Errorf("Expected dumped flashes; Got %v", flashes)
	}

	// Round 3 ----------------------------------------------------------------
	// Custom type

	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)
	rsp = NewRecorder()
	// Get a session.
	if session, err = store.Get(req, "session-key"); err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	// Get a flash.
	flashes = session.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected empty flashes; Got %v", flashes)
	}
	// Add some flashes.
	session.AddFlash(&FlashMessage{42, "foo"})
	// Save.
	if err = store.Save(req, rsp, session); err != nil {
		t.Fatalf("Error saving session: %v", err)
	}
	hdr = rsp.Header()
	cookies, ok = hdr["Set-Cookie"]
	if !ok || len(cookies) != 1 {
		t.Fatal("No cookies. Header:", hdr)
	}

	// Round 4 ----------------------------------------------------------------
	// Custom type

	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)
	req.Header.Add("Cookie", cookies[0])
	rsp = NewRecorder() // nolint
	// Get a session.
	if session, err = store.Get(req, "session-key"); err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	// Check all saved values.
	flashes = session.Flashes()
	if len(flashes) != 1 {
		t.Fatalf("Expected flashes; Got %v", flashes)
	}
	custom := flashes[0].(FlashMessage)
	if custom.Type != 42 || custom.Message != "foo" {
		t.Errorf("Expected %#v, got %#v", FlashMessage{42, "foo"}, custom)
	}

	// Round 5 ----------------------------------------------------------------
	// Check if a request shallow copy resets the request context data store.

	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)

	// Get a session.
	if session, err = store.Get(req, "session-key"); err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// Put a test value into the session data store.
	session.Values["test"] = "test-value"

	// Create a shallow copy of the request.
	req = req.WithContext(req.Context())

	// Get the session again.
	if session, err = store.Get(req, "session-key"); err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// Check if the previous inserted value still exists.
	if session.Values["test"] == nil {
		t.Fatalf("Session test value is lost in the request context!")
	}

	// Check if the previous inserted value has the same value.
	if session.Values["test"] != "test-value" {
		t.Fatalf("Session test value is changed in the request context!")
	}
}

func TestCookieStoreMapPanic(t *testing.T) {
	defer func() {
		err := recover()
		if err != nil {
			t.Fatal(err)
		}
	}()

	cfg, err := loadConfig()
	if err != nil {
		panic(err.Error())
	}
	store := NewStore(tableName, cfg, []byte("secret-key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create a new session", err)
	}

	session.Values["data"] = "hello-world"

	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to save session", err)
	}
}

func init() {
	gob.Register(FlashMessage{})
}
