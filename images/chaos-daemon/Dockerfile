FROM debian:buster-slim

ARG HTTPS_PROXY
ARG HTTP_PROXY

ENV http_proxy $HTTP_PROXY
ENV https_proxy $HTTPS_PROXY
#添加一个ps命令
RUN apt-get update && apt-get install -y tzdata procps iptables ipset stress-ng iproute2 fuse util-linux && rm -rf /var/lib/apt/lists/*

RUN update-alternatives --set iptables /usr/sbin/iptables-legacy

ADD https://github.com/riccccchard/delve_tool/releases/download/delve_tool-0.4.1/delve_tool /usr/local/bin/
#防止权限不足
RUN chmod 777 /usr/local/bin/delve_tool
ENV RUST_BACKTRACE 1

COPY --from=pingcap/chaos-binary /bin/chaos-daemon /usr/local/bin/chaos-daemon
COPY --from=pingcap/chaos-binary /bin/toda /usr/local/bin/toda
COPY --from=pingcap/chaos-binary /bin/pause /usr/local/bin/pause
COPY --from=pingcap/chaos-binary /bin/suicide /usr/local/bin/suicide
