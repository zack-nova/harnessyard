package bindings

// RedactedSensitiveValue is the stable placeholder for resolved sensitive values.
const RedactedSensitiveValue = "<redacted>"

// RedactRuntimeBindingValue returns the public display value for one resolved binding.
func RedactRuntimeBindingValue(value string, sensitive bool) string {
	if sensitive {
		return RedactedSensitiveValue
	}
	return value
}
