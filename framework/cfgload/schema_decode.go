package cfgload

// StrictDecode validates the file schema using the default schema registry and
// decodes the body into out.
func StrictDecode(path string, data []byte, out any) (any, error) {
	return DecodeWithSchema(path, data, NewSchemaRegistry(), out)
}
