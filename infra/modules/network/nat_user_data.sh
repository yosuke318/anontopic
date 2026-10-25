#!/bin/bash
# NAT インスタンスの初回起動時に実行する。IP 転送を有効にし、既定ルートの
# インターフェイスから出るパケットの送信元をこのインスタンスのアドレスに変換する。
set -euo pipefail

# t4g.nano のメモリでは、起動直後の dnf がリポジトリのメタデータを読む途中で
# OOM killer に止められる。インストールの間だけスワップファイルを足す。
fallocate -l 1G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile

dnf install -y iptables-services

swapoff /swapfile
rm -f /swapfile

systemctl enable --now iptables

echo "net.ipv4.ip_forward = 1" > /etc/sysctl.d/90-nat.conf
sysctl -p /etc/sysctl.d/90-nat.conf

iface="$(ip route show default | awk '{ print $5; exit }')"
iptables -t nat -A POSTROUTING -o "$iface" -j MASQUERADE
iptables -F FORWARD
service iptables save
