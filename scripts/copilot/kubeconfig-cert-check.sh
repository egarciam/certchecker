#!/bin/bash

# kubeconfig-cert-check.sh
# Checks the validity of certificates in a kubeconfig file

print_help() {
  echo "Usage: $0 -f <kubeconfig-file>"
  echo ""
  echo "Options:"
  echo "  -f FILE     Path to the kubeconfig file to analyze"
  echo "  -h          Show this help message"
}

check_certs() {
  local kubeconfig="$1"

  if [[ ! -f "$kubeconfig" ]]; then
    echo "Error: File '$kubeconfig' not found."
    exit 1
  fi

  echo "Checking certificates in kubeconfig: $kubeconfig"
  echo ""

  # Extract client-certificate-data and client-key-data
  contexts=$(yq e '.users[].user."client-certificate-data"' "$kubeconfig")

  if [[ -z "$contexts" ]]; then
    echo "No embedded client certificates found in kubeconfig."
    exit 0
  fi

  index=0
  for cert_data in $contexts; do
    echo "User $((++index)):"
    echo "$cert_data" | base64 --decode | openssl x509 -noout -subject -issuer -dates
    echo ""
  done
}

# Main
while getopts ":f:h" opt; do
  case ${opt} in
    f )
      kubeconfig_file="$OPTARG"
      ;;
    h )
      print_help
      exit 0
      ;;
    \? )
      echo "Invalid option: -$OPTARG" >&2
      print_help
      exit 1
      ;;
    : )
      echo "Option -$OPTARG requires an argument." >&2
      print_help
      exit 1
      ;;
  esac
done

if [[ -z "$kubeconfig_file" ]]; then
  echo "Error: kubeconfig file not specified."
  print_help
  exit 1
fi
check_certs "$kubeconfig_file"
