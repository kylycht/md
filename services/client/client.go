package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylycht/md/model"
	"github.com/nats-io/go-nats"
	"github.com/sirupsen/logrus"
)

// Service represents Client service
type Service struct {
	db       *sqlx.DB
	jsonConn *nats.EncodedConn
}

// NewService returns new instance of Client service
func NewService(db *sqlx.DB, natsClient *nats.EncodedConn) (*Service, error) {
	srv := &Service{db: db, jsonConn: natsClient}
	return srv, srv.init()
}

func (s *Service) init() error {
	if _, err := s.jsonConn.QueueSubscribe("client.add", "client-queue", s.New); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("client.get", "client-queue", s.Get); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("client.update", "client-queue", s.Update); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("client.list", "client-queue", s.List); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("client.delete", "client-queue", s.Delete); err != nil {
		return err
	}

	return nil
}

// New will perform DB insert operation for the given Client
func (s *Service) New(subject, reply string, t *model.Client) error {
	insertS := "INSERT INTO client (id, email, balance) VALUES($1, $2, $3)"

	if res, err := s.db.Exec(insertS, t.ID, t.Email, t.Balance); err != nil {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	} else if c, err := res.RowsAffected(); c == 0 || err != nil {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true})
}

// Update will perform DB update operation for the given Client
// TODO: Write better query builder using reflect package
func (s *Service) Update(subject, reply string, t *model.Client) {

	var (
		updateS  strings.Builder
		position = 1
		args     []interface{}
		update   = "UPDATE client SET "
		err      error
	)

	defer func() {
		if err != nil {
			s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
			return
		}
		s.jsonConn.Publish(reply, model.NATSMsg{Success: true})
	}()

	if _, err = updateS.WriteString(update); err != nil {
		return
	}

	if len(t.Email) > 0 {
		if _, err = updateS.WriteString(fmt.Sprintf("email=$%d ", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Email)
	}

	if t.Balance > 0 {
		if position > 1 {
			if _, err = updateS.WriteString(","); err != nil {
				return
			}
		}
		if _, err = updateS.WriteString(fmt.Sprintf("balance=$%d ", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Balance)
	}

	if update == updateS.String() {
		return
	}

	if _, err = updateS.WriteString(fmt.Sprintf("WHERE ID=$%d", position)); err != nil {
		return
	}
	args = append(args, t.ID)

	if _, err = s.db.Exec(updateS.String(), args...); err != nil {
		return
	}
}

// Delete will peform soft delete and set deleted_at datetime
func (s *Service) Delete(subject, reply, id string) error {
	if len(id) != 36 {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: model.ErrInvalidID.Error()})
	}
	delS := "UPDATE Client SET deleted_at=$1 WHERE id=$2"
	if _, err := s.db.Exec(delS, time.Now(), id); err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, &model.NATSMsg{Success: true})
}

// Get will perform DB select operation and retrieve Client by given ID
func (s *Service) Get(subject, reply, id string) error {
	if len(id) != 36 {
		logrus.WithField("id", id).Error("invalid id")
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: model.ErrInvalidID.Error()})
	}
	client := model.Client{}
	query := "SELECT * FROM client WHERE id = $1"

	if err := s.db.Get(&client, query, id); err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	d, err := json.Marshal(&client)
	if err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true, Data: d})
}

// List will perform DB select operation and retrieve all Clients by give owner(client)
func (s *Service) List(msg *nats.Msg) error {
	query := "SELECT * FROM client WHERE deleted_at IS NULL"
	clients := []model.Client{}
	if err := s.db.Select(&clients, query); err != nil {
		return s.jsonConn.Publish(msg.Reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	d, err := json.Marshal(&clients)
	if err != nil {
		return s.jsonConn.Publish(msg.Reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(msg.Reply, model.NATSMsg{Success: true, Data: d})
}
