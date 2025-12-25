package asn1

type unmarshalOpts struct {
	allowTypeGeneralString bool
}

// UnmarshalOpt describes a functional option for unmarshalling.
type UnmarshalOpt func(opts *unmarshalOpts)

// WithUnmarshalAllowTypeGeneralString preserves the type values from the field parameters when unmarshalling slices. This is an
// option since it deviates from stdlib.
func WithUnmarshalAllowTypeGeneralString(value bool) UnmarshalOpt {
	return func(opts *unmarshalOpts) {
		opts.allowTypeGeneralString = value
	}
}
