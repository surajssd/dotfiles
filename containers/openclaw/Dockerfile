FROM ghcr.io/openclaw/openclaw:latest@sha256:25f5bacf51742c3231d7dd1d319fede009a9d5aeab8b6232cd309ac6475fe82c

USER root

# Grant node user passwordless sudo, install base dependencies, and make
# /etc/passwd group-writable so the entrypoint can add the runtime UID.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    sudo \
    build-essential \
    git \
    procps \
    python3 \
    python3-pip \
    python3-venv \
    jq \
    unzip \
    iproute2 net-tools ufw firewalld nftables \
    && rm -rf /var/lib/apt/lists/* \
    && echo "node ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/node \
    && chmod 0440 /etc/sudoers.d/node \
    && chmod 0666 /etc/passwd

COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint.sh

# Install Homebrew (stays at its default /home/linuxbrew/.linuxbrew),
# then symlink from /home/node/.linuxbrew for PATH convenience.
# Then install uv and bun.
USER node
RUN NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" \
    && ln -s /home/linuxbrew/.linuxbrew /home/node/.linuxbrew \
    && curl -LsSf https://astral.sh/uv/install.sh | sh \
    && curl -fsSL https://bun.sh/install | bash
ENV PATH="/home/node/.linuxbrew/bin:/home/node/.linuxbrew/sbin:/home/node/.local/bin:/home/node/.bun/bin:${PATH}"
WORKDIR /home/node

# Make home and linuxbrew directories writable by any UID.
USER root
RUN chmod -R a+w /home/node /home/linuxbrew

# Workaround: openclaw's src/infra/exec-approvals.ts chmods /home/node/.openclaw
# on every exec tool call. macOS `container` exposes that path via VirtioFS as a
# bind mount, which rejects chmod on the mount root with EPERM. This shim
# narrows that chmod to best-effort (matches the rest of openclaw's codebase).
# The shim is injected into NODE_OPTIONS by entrypoint.sh at runtime, so that
# it survives callers (e.g. clawbox) that pass their own NODE_OPTIONS via --env.
COPY --chmod=644 openclaw-fs-shim.js /usr/local/lib/openclaw-fs-shim.js

# Copy /home/node and /home/linuxbrew to seed directories at /opt/
# (outside mount shadows) so the entrypoint can populate bind-mounted
# directories on first run.
RUN cp -a /home/node /opt/node-home-seed \
    && cp -a /home/linuxbrew /opt/linuxbrew-seed

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
