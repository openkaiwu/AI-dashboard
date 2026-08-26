package clock

import "time"

type Clock interface {
	Now() time.Time
}

type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

type Frozen struct{ T time.Time }

func (f Frozen) Now() time.Time { return f.T }
