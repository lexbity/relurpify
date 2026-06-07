package model

// Decoder decodes a config file's bytes into out.
type Decoder func(path string, data []byte, out any) (any, error)
