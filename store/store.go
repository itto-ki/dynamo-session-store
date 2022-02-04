package utils

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
)

type Store struct {
	tableName    string
	codecs       []securecookie.Codec
	dynamoClient *dynamodb.Client
}

type storeRecord struct {
	ID      string
	Values  string
	Options *sessions.Options
}

var (
	errIllegalSession     = errors.New("illegal session")
	errEncodeCookieValue  = errors.New("failed to encode a cookie value")
	errDecodeCookieValue  = errors.New("failed to decode a cookie value")
	errEncodeSessionValue = errors.New("failed to encode session values")
	errDecodeSessionValue = errors.New("failed to decode session values")
	errMarshalRecord      = errors.New("failed to marshal a dynamodb record")
	errUnmarshalRecord    = errors.New("failed to unmarshal a dynamodb record")
	errDynamoPutItem      = errors.New("failed to put item to dynamodb")
	errDynamoGetItem      = errors.New("failed to get item from dynamodb")
	errDynamoDeleteItem   = errors.New("failed to delete item from dynamodb")
)

func NewStore(tableName string, cfg aws.Config, keyPairs ...[]byte) *Store {
	return &Store{
		tableName:    tableName,
		codecs:       securecookie.CodecsFromPairs(keyPairs...),
		dynamoClient: dynamodb.NewFromConfig(cfg),
	}
}

func (s *Store) Get(r *http.Request, sessionName string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, sessionName)
}

func (s *Store) New(r *http.Request, sessionName string) (*sessions.Session, error) {
	cookie, err := r.Cookie(sessionName)
	if err != nil {
		newSession := sessions.NewSession(s, sessionName)
		newSession.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
		newSession.IsNew = true
		return newSession, nil
	}

	var sessionID string
	if err := securecookie.DecodeMulti(sessionName, cookie.Value, &sessionID, s.codecs...); err != nil {
		return nil, errDecodeCookieValue
	}
	return s.loadFromDynamo(r.Context(), sessionID)
}

func (s *Store) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	if session.Options.MaxAge < 0 {
		if err := s.deleteFromDynamo(r.Context(), session.ID); err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
		return nil
	}

	if session.ID == "" {
		return errIllegalSession
	}
	if err := s.storeToDynamo(r.Context(), session); err != nil {
		return err
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.codecs...)
	if err != nil {
		return errEncodeCookieValue
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

func (s *Store) loadFromDynamo(ctx context.Context, sessionID string) (*sessions.Session, error) {
	result, err := s.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: sessionID},
		},
	})
	if err != nil {
		return nil, errDynamoGetItem
	}
	var record storeRecord
	if err = attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, errUnmarshalRecord
	}

	decoded, err := base64.StdEncoding.DecodeString(record.Values)
	if err != nil {
		return nil, errDecodeSessionValue
	}

	var session map[interface{}]interface{}
	if err = gob.NewDecoder(bytes.NewBuffer(decoded)).Decode(&session); err != nil {
		return nil, errDecodeSessionValue
	}

	return &sessions.Session{
		ID:      record.ID,
		Values:  session,
		Options: record.Options,
		IsNew:   false,
	}, nil
}

func (s *Store) storeToDynamo(ctx context.Context, session *sessions.Session) error {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(session.Values); err != nil {
		return errEncodeSessionValue
	}
	values := base64.StdEncoding.EncodeToString(buf.Bytes())
	optionsAttributeValue, err := attributevalue.MarshalMap(session.Options)
	if err != nil {
		return errMarshalRecord
	}
	_, err = s.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item: map[string]types.AttributeValue{
			"id":      &types.AttributeValueMemberS{Value: session.ID},
			"values":  &types.AttributeValueMemberS{Value: values},
			"options": &types.AttributeValueMemberM{Value: optionsAttributeValue},
		},
	})
	if err != nil {
		return errDynamoPutItem
	}
	return nil
}

func (s *Store) deleteFromDynamo(ctx context.Context, sessionID string) error {
	_, err := s.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: sessionID},
		},
	})
	if err != nil {
		return errDynamoDeleteItem
	}
	return nil
}
