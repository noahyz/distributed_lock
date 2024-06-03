package models

type Plugin interface {
	Init() error

	TryLock() error

	Lock() error

	UnLock() error
}
