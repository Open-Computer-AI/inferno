#!/bin/sh
# Protect host services on tailscale0. Docker bindings must be audited separately.
set -eu

action=apply
if [ "${1:-}" = --remove ]; then
    action=remove
    shift
fi
if [ "$#" -ne 2 ]; then
    echo "Usage: tailnet-firewall.sh [--remove] AIR_TAILNET_IPV4 OPERATOR_TAILNET_IPV4" >&2
    exit 2
fi
for address in "$1" "$2"; do
    case "$address" in
        ''|*[!0-9.]*) echo "Expected an IPv4 address" >&2; exit 2 ;;
    esac
done

chain=INFERNO_ATTEST_TS
expected4=$(printf '%s\n' "-N $chain" \
    "-A $chain -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT" \
    "-A $chain -s $1/32 -p tcp -m tcp --dport 22 -j ACCEPT" \
    "-A $chain -s $2/32 -p tcp -m tcp --dport 22 -j ACCEPT" \
    "-A $chain -j DROP")
expected6=$(printf '%s\n' "-N $chain" \
    "-A $chain -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT" \
    "-A $chain -j DROP")

# Validate both families before modifying either. Never inherit an unexpected ACCEPT.
for command in iptables ip6tables; do
    expected=$expected4
    [ "$command" = iptables ] || expected=$expected6
    if current=$($command -w -S "$chain" 2>/dev/null); then
        if [ "$current" != "$expected" ]; then
            echo "Unexpected rules in $command $chain; refusing to change them" >&2
            exit 1
        fi
    fi
done

for command in iptables ip6tables; do
    if [ "$action" = remove ]; then
        if ! $command -w -S "$chain" >/dev/null 2>&1; then
            continue
        fi
        while $command -w -C INPUT -i tailscale0 -j "$chain" 2>/dev/null; do
            $command -w -D INPUT -i tailscale0 -j "$chain"
        done
        $command -w -D "$chain" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
        if [ "$command" = iptables ]; then
            for address in "$1" "$2"; do
                $command -w -D "$chain" -s "$address/32" -p tcp --dport 22 -j ACCEPT
            done
        fi
        $command -w -D "$chain" -j DROP
        $command -w -X "$chain"
    else
        if ! $command -w -S "$chain" >/dev/null 2>&1; then
            $command -w -N "$chain"
            $command -w -A "$chain" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
            if [ "$command" = iptables ]; then
                for address in "$1" "$2"; do
                    $command -w -A "$chain" -s "$address/32" -p tcp --dport 22 -j ACCEPT
                done
            fi
            $command -w -A "$chain" -j DROP
        fi
        $command -w -C INPUT -i tailscale0 -j "$chain" 2>/dev/null ||
            $command -w -I INPUT 1 -i tailscale0 -j "$chain"
    fi
done
