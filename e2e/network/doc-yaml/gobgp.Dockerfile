FROM alpine

LABEL author="Duan Jiong <djduanjiong@gmail.com>"

EXPOSE 179

RUN apk add curl && \
    curl -SL https://github.com/osrg/gobgp/releases/download/v2.3.0/gobgp_2.3.0_linux_amd64.tar.gz | tar xvz -C /usr/local/bin/

CMD gobgpd -l debug