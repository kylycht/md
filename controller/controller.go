package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gorilla/mux"

	"github.com/kylycht/md/model"
	nats "github.com/nats-io/go-nats"
)

var (
	timeout = time.Second * 10
)

// Controller represents REST controller
type Controller struct {
	conn *nats.EncodedConn
}

// New returns new instance of Controller
func New(conn *nats.EncodedConn) *Controller {
	return &Controller{conn: conn}
}

// CreateFreelancer handles POST /freelancer
func (c *Controller) CreateFreelancer(w http.ResponseWriter, r *http.Request) {
	var req = struct {
		Email       string `json:"email"`
		Description string `json:"description"`
		Details     string `json:"details"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(500)
		logrus.Error(err)
		return
	}
	freelancer := model.NewFreelancer(req.Email, req.Description, req.Details)
	reply := &model.NATSMsg{}

	if err := c.conn.Request("freelancer.add", freelancer, reply, timeout); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}

	if !reply.Success {
		logrus.Error(reply.Message)
		w.WriteHeader(500)
		return
	}
	w.Write([]byte(`{"id":"` + freelancer.ID + `"}`))
}

// GetFreelancer handles GET /freelancer/{id}
func (c *Controller) GetFreelancer(w http.ResponseWriter, r *http.Request) {
	var freelancer model.Freelancer
	params := mux.Vars(r)
	freelancer.ID = params["id"]
	reply := &model.NATSMsg{}

	if err := c.conn.Request("freelancer.get", freelancer.ID, reply, timeout); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}

	if !reply.Success {
		logrus.Error(reply.Message)
		w.WriteHeader(500)
		return
	}
	w.Write(reply.Data)
}

// GetClient handles GET /client/{id}
func (c *Controller) GetClient(w http.ResponseWriter, r *http.Request) {
	var client model.Client
	if err := json.NewDecoder(r.Body).Decode(&client); err != nil {
	}

	if client.ID == "" {
		params := mux.Vars(r)
		client.ID = params["id"]
	}

	if client.ID == "" {
		w.WriteHeader(500)
		return
	}
	reply := &model.NATSMsg{}

	if err := c.conn.Request("client.get", client.ID, reply, timeout); err != nil {
		w.WriteHeader(500)
		return
	}

	if !reply.Success {
		w.WriteHeader(500)
		return
	}
	w.Write(reply.Data)
}

// GetTask handles GET /task/{id}
func (c *Controller) GetTask(w http.ResponseWriter, r *http.Request) {
	var task model.Task
	params := mux.Vars(r)
	task.ID = params["id"]

	if task.ID == "" {
		w.WriteHeader(500)
		logrus.Error("missing ID")
		return
	}
	reply := &model.NATSMsg{}

	if err := c.conn.Request("task.get", task.ID, reply, timeout); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}

	if !reply.Success {
		logrus.Error(reply.Message)
		w.WriteHeader(500)
		return
	}
	w.Write(reply.Data)
}

// CreateTask handles POST /task
func (c *Controller) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req = struct {
		ClientID    string `json:"client_id"`
		Description string `json:"description"`
		Deadline    int64  `json:"deadline"`
		Fee         int64  `json:"fee"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}
	task := model.NewTask(time.Duration(req.Deadline)*time.Second, req.Fee, req.ClientID, req.Description)
	reply := &model.NATSMsg{}
	if err := c.conn.Request("task.add", task, reply, timeout); err != nil {
		w.WriteHeader(500)
		logrus.WithField("endpoint", "task.add").WithField("msg", reply.Message).Error(err)
		return
	}

	if !reply.Success {
		logrus.WithField("msg", reply.Message).Error()
		w.WriteHeader(500)
		return
	}
	w.Write([]byte(`{"id":"` + task.ID + `"}`))
}

// UpdateTask handles PUT /task/{id}
func (c *Controller) UpdateTask(w http.ResponseWriter, r *http.Request) {
	var task model.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}
	logrus.Info(task.FreelancerID)
	if task.ID == "" {
		params := mux.Vars(r)
		task.ID = params["id"]
	}
	if task.ID == "" {
		logrus.Error("missing ID")
		w.WriteHeader(500)
		return
	}

	reply := &model.NATSMsg{}
	if err := c.conn.Request("task.update", task, reply, timeout); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}

	if !reply.Success {
		logrus.Error(reply.Message)
		w.WriteHeader(500)
		return
	}

	w.Write([]byte(`{"id":"` + task.ID + `"}`))
}

// CreateClient handles POST /client
func (c *Controller) CreateClient(w http.ResponseWriter, r *http.Request) {
	var client model.Client
	if err := json.NewDecoder(r.Body).Decode(&client); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}
	client.ID = model.NewID()
	reply := &model.NATSMsg{}

	if err := c.conn.Request("client.add", client, reply, timeout); err != nil {
		logrus.Error(err)
		w.WriteHeader(500)
		return
	}

	if !reply.Success {
		logrus.Error(reply.Message)
		w.WriteHeader(500)
		return
	}
	w.Write([]byte(`{"id":"` + client.ID + `"}`))
}
