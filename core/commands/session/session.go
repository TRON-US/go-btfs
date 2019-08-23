package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

var SessionMap map[string]*Session

type Session struct {
	sync.Mutex

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

func (s *Session) NewSession(ssID string, fileHash string) error {
	s.Lock()
	defer s.Unlock()

	session := &Session{
		Time:     time.Now(),
		Status:   "init",
		FileHash: fileHash,
	}
	SessionMap[ssID] = session
	return nil
}

func (s *Session) SetChallenge(ssID string, challengeID string, challengeHash string)  {
	challenge := make(map[string]interface{})
	challenge[challengeID] = challengeHash
	SessionMap[ssID].Challenge = challenge
	SessionMap[ssID].Time = time.Now()
}
