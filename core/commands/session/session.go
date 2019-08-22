package session

import (
	"time"

	"github.com/google/uuid"
)

var SessionMap map[string]*Session

type Session struct {
	Time      time.Time
	FileHash  string
	Status    string
	Chunks    map[string]interface{}
	Challenge map[string]interface{}
}

func init() {
	SessionMap = make(map[string]*Session)
}

func NewSessionID() (string, error) {
	seid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return seid.String(), nil
}

func NewSession(ssid string, fileHash string) (*Session, error) {
	time := time.Now()
	//challenge := make(map[string]interface{})
	session := &Session{
		Time:     time,
		Status:   "init",
		FileHash: fileHash,
	}
	SessionMap[ssid] = session
	return session, nil
}
