package main

import (
	"log"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/jmoiron/sqlx"

	"github.com/gorilla/mux"
	"github.com/kylycht/md/controller"
	"github.com/kylycht/md/services/client"
	"github.com/kylycht/md/services/freelancer"
	"github.com/kylycht/md/services/task"
	"github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	nats "github.com/nats-io/go-nats"
)

var (
	taskSrv = &task.Service{}
	fSrv    = &freelancer.Service{}
	cSrv    = &client.Service{}
	logger  = logrus.New()
)

func main() {
	go runServer().Start()

	ds := "dbname=bar sslmode=disable"
	if c, ok := os.LookupEnv("DB_CONN"); ok {
		ds = c
	}
	db, err := sqlx.Connect("postgres", ds)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	initDB(db)
	// connect to nats
	natsConn, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		log.Fatal(err)
	}
	if !natsConn.IsConnected() {
		log.Fatal("no nats connection")
	}
	// init nats JSON encoded connection
	natsEncConn, err := nats.NewEncodedConn(natsConn, nats.JSON_ENCODER)
	if err != nil {
		log.Fatal(err)
	}
	//task service
	taskSrv, err = task.NewService(db, natsEncConn)
	if err != nil {
		log.Fatal(err)
	}
	//freelancer service
	fSrv, err = freelancer.NewService(db, natsEncConn)
	if err != nil {
		log.Fatal(err)
	}
	// client service
	cSrv, err = client.NewService(db, natsEncConn)
	if err != nil {
		log.Fatal(err)
	}
	//controller
	ctrl := controller.New(natsEncConn)
	router := mux.NewRouter()

	router.HandleFunc("/client", ctrl.CreateClient).Methods("POST")
	router.HandleFunc("/client/{id}", ctrl.GetClient).Methods("GET")

	router.HandleFunc("/task", ctrl.CreateTask).Methods("POST")
	router.HandleFunc("/task/{id}", ctrl.GetTask).Methods("GET")
	router.HandleFunc("/task/{id}", ctrl.UpdateTask).Methods("PUT")

	router.HandleFunc("/freelancer", ctrl.CreateFreelancer).Methods("POST")
	router.HandleFunc("/freelancer/{id}", ctrl.GetFreelancer)

	log.Fatal(http.ListenAndServe(":8000", router))
}

func runServer() *server.Server {
	return gnatsd.RunDefaultServer()
}

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

func initDB(db *sqlx.DB) error {

	db.Exec(taskSchema)
	db.Exec(billingSchema)
	db.Exec(clientSchema)
	db.Exec(freelancerSchema)

	return nil
}
