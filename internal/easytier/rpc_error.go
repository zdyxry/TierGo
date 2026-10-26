package easytier

func rpcErrorDetail(data []byte) string {
	fields, err := wireFields(data)
	if err != nil {
		return "malformed error"
	}
	for _, field := range fields {
		nested, err := wireFields(field.bytes)
		if err != nil {
			continue
		}
		for _, value := range nested {
			if value.number == 1 && len(value.bytes) > 0 {
				return redact(string(value.bytes))
			}
		}
	}
	return "unspecified RPC error"
}
