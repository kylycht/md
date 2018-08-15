package task

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylycht/md/model"
	"github.com/kylycht/md/services/freelancer"
	_ "github.com/lib/pq"
	"github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	nats "github.com/nats-io/go-nats"
)

var s *Service

/*

 */

var taskSchema = `CREATE TABLE TASK (
    ID varchar(36) PRIMARY KEY NOT NULL,
    CLIENT_ID varchar(36) NOT NULL,
    FREELANCER_ID varchar(36),
    DESCRIPTION text,
    FEE bigint,
	STATUS varchar,
	Deadline int8,
    CREATED_AT timestamp,
    STARTED_AT timestamp,
    DELETED_AT timestamp,
    UPDATED_AT timestamp
)`

var billingSchema = `CREATE TABLE BILLING (
	ID varchar(36) PRIMARY KEY NOT NULL,
	CLIENT_ID varchar(36) NOT NULL,
	FREELANCER_ID varchar(36),
	PAID_DATE timestamp,
	STATUS varchar,
	AMOUNT bigint,
	TASK_ID varchar(36)
)`

var clientSchema = `CREATE TABLE CLIENT (
	ID varchar(36) PRIMARY KEY NOT NULL,
	BALANCE bigint,
	EMAIL varchar(128),
    DELETED_AT timestamp
)`

var freelancerSchema = `CREATE TABLE FREELANCER (
    ID varchar(36) PRIMARY KEY NOT NULL,
	DESCRIPTION text,
	DETAILS text,
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

	s.db.Exec(taskSchema)
	s.db.Exec(billingSchema)
	s.db.Exec(clientSchema)
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
	s.jsonConn.Subscribe("client.get", func(subject, reply, id string) {
		s.jsonConn.Publish(reply, model.Client{ID: "74dcc973-50a9-4b87-9403-814a69c5359e", Balance: 123456789})
	})
	// subscribe to topics
	s.init()
	_, err = freelancer.NewService(db, natsEncConn)
	if err != nil {
		t.Error(err)
	}

	return func() {
		// s.db.Exec("DROP TABLE task")
		s.db.Close()
		natsServer.Shutdown()
	}
}

func populateDB(t *testing.T) string {
	task := NewTask()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("task.add", task, reply, time.Second*5); err != nil {
		t.Error(err)
		return ""
	}
	return task.ID
}

func NewTask() model.Task {
	return model.NewTask(time.Hour*24*2, 133227, model.NewID(), "foo bar")
}

func TestFlow(t *testing.T) {
	destroy := setUp(t)
	defer destroy()
	task := NewTask()
	reply := &model.NATSMsg{}
	// Add new task
	if err := s.jsonConn.Request("task.add", task, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}
	freelancer := model.NewFreelancer("freelancer@email.com", "dev", "python")
	task.FreelancerID = freelancer.ID

	if err := s.jsonConn.Request("freelancer.add", &freelancer, reply, timeout); err != nil {
		t.Error(err)
		return
	}
	if !reply.Success {
		t.Error(reply.Message)
		return
	}
	// Assign developer to a task
	if err := s.jsonConn.Request("task.update", task, reply, timeout); err != nil {
		t.Error(err)
		return
	}
	if !reply.Success {
		t.Error(reply.Message)
		return
	}
	// Close task
	task.Status = model.Closed
	// Assign developer to a task
	if err := s.jsonConn.Request("task.update", task, reply, timeout); err != nil {
		t.Error(err)
		return
	}
	if !reply.Success {
		t.Error(reply.Message)
		return
	}
}

func TestService_Create(t *testing.T) {
	destroy := setUp(t)
	defer destroy()
	task := NewTask()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("task.add", task, reply, time.Second*10); err != nil {
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

	task := NewTask()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("task.add", task, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	err := s.jsonConn.Request("task.get", task.ID, reply, time.Second*10)
	if err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error("unable to retrieve task by id: ", reply.Message)
		return
	}

	task2 := &model.Task{}
	if err := json.Unmarshal(reply.Data, task2); err != nil {
		t.Error(err, string(reply.Data))
		return
	}

	if task2.ID != task.ID {
		t.Errorf("task ID mismatch, expected=%s got=%s", task.ID, task2.ID)
		return
	}

	if task2.Description != task.Description {
		t.Errorf("task description mismatch, expected=%s vs got=%s", task.Description, task2.Description)
		return
	}
}

func TestService_Delete(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	task := NewTask()
	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("task.add", task, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if err := s.jsonConn.Request("task.delete", task.ID, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error("unable to delete task by id: ", reply.Message)
		return
	}
}

func TestService_List(t *testing.T) {
	destroy := setUp(t)
	defer destroy()

	clientID := model.NewID()

	task := NewTask()
	task.ClientID = clientID

	reply := &model.NATSMsg{}
	if err := s.jsonConn.Request("task.add", task, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	task2 := NewTask()
	task2.ClientID = clientID

	if err := s.jsonConn.Request("task.add", task2, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}
	err := s.jsonConn.Request("task.list", "", reply, time.Second*10)
	if err != nil {
		t.Error(err)
		return
	}
	tasks := []model.Freelancer{}
	if err := json.Unmarshal(reply.Data, &tasks); err != nil {
		t.Error(err)
		return
	}
	if len(tasks) != 2 {
		t.Error("expected 2, got: ", len(tasks))
		return
	}
	if len(tasks) != 2 {
		t.Errorf("expected=%d, got=%d", 2, len(tasks))
	}
}

func TestService_Update(t *testing.T) {
	destroy := setUp(t)
	defer destroy()
	task := NewTask()
	reply := &model.NATSMsg{}

	if err := s.jsonConn.Request("task.add", task, reply, time.Second*10); err != nil {
		t.Error(err)
		return
	}

	if !reply.Success {
		t.Error(reply.Message)
		return
	}
	taskID := task.ID
	type args struct {
		t model.Task
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "no-update",
			args:    args{t: model.Task{ID: taskID}},
			wantErr: false,
		},
		{
			name:    "update-description-only",
			args:    args{t: model.Task{ID: taskID, Description: "bar foo"}},
			wantErr: false,
		},
		{
			name:    "update-desc-fee",
			args:    args{t: model.Task{ID: taskID, Description: "bar foo buzz", Fee: 99999}},
			wantErr: false,
		},
		{
			name:    "update-duration",
			args:    args{t: model.Task{ID: taskID, Deadline: time.Hour * 60 * 7}},
			wantErr: false,
		},
		{
			name:    "update-status",
			args:    args{t: model.Task{ID: taskID, Status: model.Closed}},
			wantErr: false,
		},
		{
			name:    "update-task-id",
			args:    args{t: model.Task{ID: "46f5974b-e939-4017-b239-56120c0d00bb", FreelancerID: model.NewID(), Status: model.Closed}},
			wantErr: false,
		},
		{
			name:    "update-all",
			args:    args{t: model.Task{ID: taskID, FreelancerID: model.NewID(), Description: "bar foo buzzfoo", Fee: 29999}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := &model.NATSMsg{}
			if err := s.jsonConn.Request("task.update", &tt.args.t, reply, time.Second*10); (err != nil) != tt.wantErr {
				t.Errorf("Service.Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reply.Success {
				t.Error(reply.Message)
				return
			}
		})
	}
}
