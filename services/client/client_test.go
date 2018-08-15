package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylycht/md/model"
	"github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	nats "github.com/nats-io/go-nats"
)

var s *Service

/*

 */

var clientSchema = `CREATE TABLE CLIENT (
	ID varchar(36) PRIMARY KEY NOT NULL,
	BALANCE bigint,
	EMAIL varchar(128),
    DELETED_AT timestamp
)`

func startServer() *server.Server {
	return gnatsd.RunDefaultServer()
}

func setUp(t *testing.T) func() {
	s = &Service{}
	db, err := sqlx.Connect("postgres", "dbname=bar sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}

	s.db = db
	s.db.Exec(clientSchema)

	natsServer := startServer()

	natsConn, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		t.Fatal(err)
	}
	if !natsConn.IsConnected() {
		t.Fatal("no nats connection")
	}

	natsEncConn, err := nats.NewEncodedConn(natsConn, nats.JSON_ENCODER)
	if err != nil {
		t.Fatal(err)
	}
	s.jsonConn = natsEncConn

	// subscribe to topics
	s.init()
	return func() {
		// s.db.Exec("DROP TABLE client")
		s.db.Close()
		natsServer.Shutdown()
	}
}

func populateDB(t *testing.T) string {
	client := NewClient()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("client.add", client, reply, time.Second*5); err != nil {
		t.Error(err)
		return ""
	}

	return client.ID
}

func NewClient() model.Client {
	return model.NewClient("imail@email.com", 123456789)
}

func TestService_Create(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	client := NewClient()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("client.add", client, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error(reply.Message)
		return
	}
}

func TestService_Get(t *testing.T) {
	d := setUp(t)
	defer d()

	client := NewClient()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("client.add", client, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	err := s.jsonConn.Request("client.get", client.ID, reply, time.Second*10)
	if err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error("unable to retrieve client by id: ", reply.Message)
		return
	}

	lancer := &model.Client{}
	if err := json.Unmarshal(reply.Data, lancer); err != nil {
		t.Error(err, string(reply.Data))
		return
	}
	if lancer.ID != client.ID {
		t.Errorf("client ID mismatch, expected=%s got=%s", client.ID, lancer.ID)
		return
	}
}

func TestService_Delete(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	client := NewClient()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("client.add", client, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if err := s.jsonConn.Request("client.delete", client.ID, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error("unable to delete client by id: ", reply.Message)
		return
	}
}

func TestService_List(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	client := NewClient()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("client.add", client, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}
	client2 := model.NewClient("aaa@email.com", 99999999)

	if err := s.jsonConn.Request("client.add", client2, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	err := s.jsonConn.Request("client.list", "", reply, time.Second*10)
	if err != nil {
		t.Error(err)
		return
	}
	clients := []model.Client{}
	if err := json.Unmarshal(reply.Data, &clients); err != nil {
		t.Error(err)
		return
	}
	if len(clients) != 2 {
		t.Error("expected 2, got: ", len(clients))
		return
	}
}

func TestService_Update(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	client := NewClient()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("client.add", client, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error(reply.Message)
		return
	}

	clientID := client.ID

	type args struct {
		t model.Client
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "no-update",
			args:    args{t: model.Client{ID: clientID}},
			wantErr: false,
		},
		{
			name:    "update-email",
			args:    args{t: model.Client{ID: clientID, Email: "hello@foo.com"}},
			wantErr: false,
		},
		{
			name:    "update-balance",
			args:    args{t: model.Client{ID: clientID, Balance: 33333}},
			wantErr: false,
		},
		{
			name: "update-all",
			args: args{
				t: model.Client{
					ID:      clientID,
					Email:   "noemail@email.com",
					Balance: 121313121,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := &model.NATSMsg{}
			if err := s.jsonConn.Request("client.update", &tt.args.t, reply, time.Second*10); (err != nil) != tt.wantErr {
				t.Errorf("Service.Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reply.Success {
				t.Error(reply.Message)
				return
			}
		})
	}
}
