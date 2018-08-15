package freelancer

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylycht/md/model"
	"github.com/nats-io/go-nats"
)

// Service represents Freelancer service
type Service struct {
	db       *sqlx.DB
	jsonConn *nats.EncodedConn
}

// NewService returns new instance of Freelancer service
func NewService(db *sqlx.DB, natsClient *nats.EncodedConn) (*Service, error) {
	srv := &Service{db: db, jsonConn: natsClient}
	return srv, srv.init()
}

func (s *Service) init() error {

	if _, err := s.jsonConn.QueueSubscribe("freelancer.add", "freelancer-queue", s.New); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("freelancer.get", "freelancer-queue", s.Get); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("freelancer.update", "freelancer-queue", s.Update); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("freelancer.list", "freelancer-queue", s.List); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("freelancer.delete", "freelancer-queue", s.Delete); err != nil {
		return err
	}

	return nil
}

// New will perform DB insert operation for the given Freelancer
func (s *Service) New(subject, reply string, t *model.Freelancer) error {
	insertS := "INSERT INTO freelancer (id, description,details, email) VALUES($1, $2, $3, $4)"

	if res, err := s.db.Exec(insertS, t.ID, t.Description, t.Details, t.Email); err != nil {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	} else if c, err := res.RowsAffected(); c == 0 || err != nil {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true})
}

// Update will perform DB update operation for the given Freelancer
// TODO: Write better query builder using reflect package
func (s *Service) Update(subject, reply string, t *model.Freelancer) {

	var (
		updateS  strings.Builder
		position = 1
		args     []interface{}
		update   = "UPDATE freelancer SET "
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

	if t.Details.Valid {
		if _, err = updateS.WriteString(fmt.Sprintf("description=$%d ", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Description)
	}

	if len(t.Email) > 0 {
		if position > 1 {
			if _, err = updateS.WriteString(", "); err != nil {
				return
			}
		}
		if _, err = updateS.WriteString(fmt.Sprintf("email=$%d ", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Email)
	}

	if t.Details.Valid {
		if position > 1 {
			if _, err = updateS.WriteString(", "); err != nil {
				return
			}
		}
		if _, err = updateS.WriteString(fmt.Sprintf("details=$%d ", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Details)
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
	delS := "UPDATE Freelancer SET deleted_at=$1 WHERE id=$2"
	if _, err := s.db.Exec(delS, time.Now(), id); err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, &model.NATSMsg{Success: true})
}

// Get will perform DB select operation and retrieve Freelancer by given ID
func (s *Service) Get(subject, reply, id string) error {
	if len(id) != 36 {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: model.ErrInvalidID.Error()})
	}
	freelancer := model.Freelancer{}
	query := "SELECT * FROM freelancer WHERE id = $1"

	if err := s.db.Get(&freelancer, query, id); err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	d, err := json.Marshal(&freelancer)
	if err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true, Data: d})
}

// List will perform DB select operation and retrieve all Freelancers by give owner(client)
func (s *Service) List(msg *nats.Msg) error {
	query := "SELECT * FROM freelancer WHERE deleted_at IS NULL"
	freelancers := []model.Freelancer{}
	if err := s.db.Select(&freelancers, query); err != nil {
		return s.jsonConn.Publish(msg.Reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	d, err := json.Marshal(&freelancers)
	if err != nil {
		return s.jsonConn.Publish(msg.Reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(msg.Reply, model.NATSMsg{Success: true, Data: d})
}
