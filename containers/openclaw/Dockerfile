FROM ghcr.io/openclaw/openclaw:latest

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
    iproute2 net-tools ufw firewalld nftables \
    && rm -rf /var/lib/apt/lists/* \
    && echo "node ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/node \
    && chmod 0440 /etc/sudoers.d/node \
    && chgrp node /etc/passwd && chmod 0664 /etc/passwd

COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint.sh

# Install Homebrew (hardcoded to /home/linuxbrew/.linuxbrew), then move
# it to /home/node/.linuxbrew and symlink the old path for compatibility.
# Then install uv and bun.
USER node
RUN NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" \
    && sudo mv /home/linuxbrew/.linuxbrew /home/node/.linuxbrew \
    && sudo ln -s /home/node/.linuxbrew /home/linuxbrew/.linuxbrew \
    && curl -LsSf https://astral.sh/uv/install.sh | sh \
    && curl -fsSL https://bun.sh/install | bash
ENV PATH="/home/node/.linuxbrew/bin:/home/node/.linuxbrew/sbin:/home/node/.local/bin:/home/node/.bun/bin:${PATH}"
WORKDIR /home/node

# Make home directory and tool caches writable by any UID.
USER root
RUN chmod -R a+w /home/node

# Copy /home/node to a seed directory at /opt/ (outside the home mount shadow)
# so the entrypoint can populate a bind-mounted /home/node on first run.
RUN cp -a /home/node /opt/node-home-seed

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
