package lib

func ConvertInt32ToIntPointer(i *int32) *int {
	if i == nil {
		return nil
	}
	v := int(*i)
	return &v
}

func ConvertInt64ToIntPointer(i *int64) *int64 {
	if i == nil {
		return nil
	}
	v := *i
	return &v
}

func ConvertBoolToBoolPointer(b *bool) *bool {
	if b == nil {
		return nil
	}
	v := *b
	return &v
}

func ConvertStringToStringPointer(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}
