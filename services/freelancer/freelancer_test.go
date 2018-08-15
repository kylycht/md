package freelancer

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylycht/md/model"
	_ "github.com/lib/pq"
	"github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	nats "github.com/nats-io/go-nats"
)

var s *Service

/*

 */

var freelancerSchema = `CREATE TABLE FREELANCER (
    ID varchar(36) PRIMARY KEY NOT NULL,
	DESCRIPTION text,
	DETAILS text,
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
	s.db.Exec(freelancerSchema)

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
		// s.db.Exec("DROP TABLE freelancer")
		s.db.Close()
		natsServer.Shutdown()
	}
}

func populateDB(t *testing.T) string {
	freelancer := NewFreelancer()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("freelancer.add", freelancer, reply, time.Second*5); err != nil {
		t.Error(err)
		return ""
	}

	return freelancer.ID
}

func NewFreelancer() model.Freelancer {
	return model.NewFreelancer("imail@email.com", "dev", "python")
}

func TestService_Create(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	freelancer := NewFreelancer()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("freelancer.add", freelancer, reply, time.Second*10); err != nil {
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

	freelancer := NewFreelancer()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("freelancer.add", freelancer, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	err := s.jsonConn.Request("freelancer.get", freelancer.ID, reply, time.Second*10)
	if err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error("unable to retrieve freelancer by id: ", reply.Message)
		return
	}

	lancer := &model.Freelancer{}
	if err := json.Unmarshal(reply.Data, lancer); err != nil {
		t.Error(err, string(reply.Data))
		return
	}

	if lancer.ID != freelancer.ID {
		t.Errorf("freelancer ID mismatch, expected=%s got=%s", freelancer.ID, lancer.ID)
		return
	}

	if lancer.Description != freelancer.Description {
		t.Errorf("freelancer description mismatch, expected=%s vs got=%s", freelancer.Description.String, lancer.Description.String)
		return
	}
}

func TestService_Delete(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	freelancer := NewFreelancer()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("freelancer.add", freelancer, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if err := s.jsonConn.Request("freelancer.delete", freelancer.ID, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error("unable to delete freelancer by id: ", reply.Message)
		return
	}
}

func TestService_List(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	freelancer := NewFreelancer()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("freelancer.add", freelancer, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}
	freelancer2 := model.NewFreelancer("aaa@email.com", "description here", "details here")

	if err := s.jsonConn.Request("freelancer.add", freelancer2, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	err := s.jsonConn.Request("freelancer.list", "", reply, time.Second*10)
	if err != nil {
		t.Error(err)
		return
	}
	freelancers := []model.Freelancer{}
	if err := json.Unmarshal(reply.Data, &freelancers); err != nil {
		t.Error(err)
		return
	}
	if len(freelancers) != 2 {
		t.Error("expected 2, got: ", len(freelancers))
		return
	}
}

func TestService_Update(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	freelancer := NewFreelancer()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("freelancer.add", freelancer, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error(reply.Message)
		return
	}

	freelancerID := freelancer.ID

	type args struct {
		t model.Freelancer
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "no-update",
			args:    args{t: model.Freelancer{ID: freelancerID}},
			wantErr: false,
		},
		{
			name:    "update-description-only",
			args:    args{t: model.Freelancer{ID: freelancerID, Description: sql.NullString{String: "bar foo", Valid: true}}},
			wantErr: false,
		},
		{
			name:    "update-desc-details",
			args:    args{t: model.Freelancer{ID: freelancerID, Description: sql.NullString{String: "bar foo buzz", Valid: true}, Details: sql.NullString{String: "dett", Valid: true}}},
			wantErr: false,
		},
		{
			name: "update-all",
			args: args{
				t: model.Freelancer{
					ID:          freelancerID,
					Description: sql.NullString{String: "bar foo buzz", Valid: true},
					Details:     sql.NullString{String: "dett", Valid: true},
					Email:       "noemail@email.com",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := &model.NATSMsg{}
			if err := s.jsonConn.Request("freelancer.update", &tt.args.t, reply, time.Second*10); (err != nil) != tt.wantErr {
				t.Errorf("Service.Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reply.Success {
				t.Error(reply.Message)
				return
			}
		})
	}
}
