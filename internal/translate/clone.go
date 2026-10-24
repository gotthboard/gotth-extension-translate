package translate

func cloneRuntime(in RuntimeConfig) RuntimeConfig {
	out := in
	out.Manifest.Protocols = cloneSlice(in.Manifest.Protocols)
	out.Manifest.Interfaces = cloneSlice(in.Manifest.Interfaces)
	out.Manifest.Capabilities = cloneSlice(in.Manifest.Capabilities)
	out.Manifest.Secrets = cloneSlice(in.Manifest.Secrets)
	out.Grant.Capabilities = cloneSlice(in.Grant.Capabilities)
	out.Grant.Interfaces = cloneSlice(in.Grant.Interfaces)
	out.Grant.Secrets = cloneSlice(in.Grant.Secrets)
	out.Session.Interfaces = cloneSlice(in.Session.Interfaces)
	out.Session.Capabilities = cloneSlice(in.Session.Capabilities)
	out.Session.Secrets = cloneSlice(in.Session.Secrets)
	return out
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}
