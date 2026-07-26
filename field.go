package evo

// Int builds an integer diagnostic field.
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// String builds a string diagnostic field.
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Duration builds a duration diagnostic field.
func Duration(key string, value interface{ String() string }) Field {
	return Field{Key: key, Value: value.String()}
}
