package asn1

type marshalOpts struct {
	slicePreserveTypes  bool
	sliceAllowStrings   bool
	generalStringOctets bool
}

// MarshalOpt describes a functional option for marshalling.
type MarshalOpt func(opts *marshalOpts)

// WithMarshalSlicePreserveTypes preserves the type values from the field parameters when unmarshalling slices. This is an
// option since it deviates from stdlib.
func WithMarshalSlicePreserveTypes(value bool) MarshalOpt {
	return func(opts *marshalOpts) {
		opts.slicePreserveTypes = value
	}
}

// WithMarshalSliceAllowStrings allows slices of strings when unmarshalling slices. This is an option since it deviates from
// stdlib.
func WithMarshalSliceAllowStrings(value bool) MarshalOpt {
	return func(opts *marshalOpts) {
		opts.sliceAllowStrings = value
	}
}

// WithMarshalGeneralStringOctets marshals strings with the general type as their octets unchanged, as the matching
// unmarshal does. By default a GeneralString may only contain IA5 (ASCII) characters. Kerberos implementations commonly
// carry UTF-8 in a KerberosString, which RFC 4120 section 5.2.1 permits but warns is an interoperability risk, so this
// is an option rather than the default.
func WithMarshalGeneralStringOctets(value bool) MarshalOpt {
	return func(opts *marshalOpts) {
		opts.generalStringOctets = value
	}
}
