package typeonly

// A defines the a type.
type A struct {
	X int
	Y string
}

// B defines the b type.
type B interface {
	Method() string
}

// Version defines a constant for version.
const Version = "1.0.0"
