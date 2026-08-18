import ssl

create_default_context = ssl.create_default_context


def create_legacy_ca_compatible_context(*args, **kwargs):
    context = create_default_context(*args, **kwargs)
    context.verify_flags &= ~ssl.VERIFY_X509_STRICT
    return context


# Remove this workaround when Netskope supplies an RFC 5280 CA chain with keyUsage.
ssl.create_default_context = create_legacy_ca_compatible_context
