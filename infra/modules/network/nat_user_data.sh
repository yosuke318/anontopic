#!/bin/bash
# NAT インスタンスの初回起動時に実行する。IP 転送を有効にし、既定ルートの
# インターフェイスから出るパケットの送信元をこのインスタンスのアドレスに変換する。
set -euo pipefail

dnf install -y iptables-services
systemctl enable --now iptables

echo "net.ipv4.ip_forward = 1" > /etc/sysctl.d/90-nat.conf
sysctl -p /etc/sysctl.d/90-nat.conf

iface="$(ip route show default | awk '{ print $5; exit }')"
iptables -t nat -A POSTROUTING -o "$iface" -j MASQUERADE
iptables -F FORWARD
service iptables save
