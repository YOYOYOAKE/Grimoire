package id

import "github.com/google/uuid"

type Generator interface {
	NewString() string
}

type UUIDGenerator struct{}

type StaticGenerator struct {
	value string
}

func NewUUIDGenerator() UUIDGenerator {
	return UUIDGenerator{}
}

func NewStaticGenerator(value string) StaticGenerator {
	return StaticGenerator{value: value}
}

func (UUIDGenerator) NewString() string {
	return uuid.NewString()
}

func (g StaticGenerator) NewString() string {
	return g.value
}
