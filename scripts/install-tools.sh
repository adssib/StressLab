#!/usr/bin/env bash
# Installs the laptop-side tools StressLab needs on Ubuntu 24.04 (WSL works):
# Azure CLI, Terraform, k6 and jq. Ansible and Go are checked, not installed.
# Commands follow each tool's official install page (checked 2026-10-03).
# Run with: sudo bash scripts/install-tools.sh
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "run with sudo: sudo bash $0" >&2
  exit 1
fi

apt-get update
apt-get install -y apt-transport-https ca-certificates curl gnupg lsb-release wget jq

# Azure CLI — https://learn.microsoft.com/cli/azure/install-azure-cli-linux?pivots=apt
mkdir -p /etc/apt/keyrings
curl -sLS https://packages.microsoft.com/keys/microsoft.asc |
  gpg --dearmor --yes -o /etc/apt/keyrings/microsoft.gpg
chmod go+r /etc/apt/keyrings/microsoft.gpg
cat > /etc/apt/sources.list.d/azure-cli.sources <<EOF
Types: deb
URIs: https://packages.microsoft.com/repos/azure-cli/
Suites: $(lsb_release -cs)
Components: main
Architectures: $(dpkg --print-architecture)
Signed-by: /etc/apt/keyrings/microsoft.gpg
EOF

# Terraform — https://developer.hashicorp.com/terraform/install
wget -qO - https://apt.releases.hashicorp.com/gpg |
  gpg --dearmor --yes -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" \
  > /etc/apt/sources.list.d/hashicorp.list

# k6 — https://grafana.com/docs/k6/latest/set-up/install-k6/
curl -fsSL https://dl.k6.io/key.gpg |
  gpg --dearmor --yes -o /usr/share/keyrings/k6-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  > /etc/apt/sources.list.d/k6.list

apt-get update
apt-get install -y azure-cli terraform k6

echo
echo "--- installed ---"
az version --query '"azure-cli"' -o tsv | sed 's/^/az        /'
terraform version | head -1
k6 version
jq --version
command -v ansible >/dev/null && ansible --version | head -1 || echo "ansible   missing: sudo apt-get install -y ansible"
command -v go >/dev/null && go version || echo "go        missing"
