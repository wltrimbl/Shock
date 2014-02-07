// Package db to connect to mongodb
package db

import (
	"errors"
	"fmt"
	"github.com/MG-RAST/Shock/shock-server/conf"
	"labix.org/v2/mgo"
	"time"
)

const (
	DbTimeout = time.Duration(time.Second * conf.Conf["mongodb-timeout"])
)

var (
	Connection connection
)

type connection struct {
	dbname   string
	username string
	password string
	Session  *mgo.Session
	DB       *mgo.Database
}

func Initialize() (err error) {
	c := connection{}
	s, err := mgo.DialWithTimeout(conf.Conf["mongodb-hosts"], DbTimeout)
	if err != nil {
		e := errors.New(fmt.Sprintf("no reachable mongodb server(s) at %s", conf.Conf["mongodb-hosts"]))
		return e
	}
	c.Session = s
	c.DB = c.Session.DB(conf.Conf["mongodb-database"])
	if conf.Conf["mongodb-user"] != "" && conf.Conf["mongodb-password"] != "" {
		c.DB.Login(conf.Conf["mongodb-user"], conf.Conf["mongodb-password"])
	}
	Connection = c
	return
}

func Drop() error {
	return Connection.DB.DropDatabase()
}
