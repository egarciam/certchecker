#!/bin/bash

# Function to display help menu
show_help() {
    echo "Usage: $0 [KUBECONFIG_PATH]"
    echo ""
    echo "This script checks the expiration dates of all certificates in a kubeconfig file."
    echo ""
    echo "Arguments:"
    echo "  KUBECONFIG_PATH  Path to the kubeconfig file (default: ~/.kube/config)"
    echo ""
    echo "Options:"
    echo "  -h, --help      Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                  # Check default kubeconfig"
    echo "  $0 ~/.kube/config   # Check specific kubeconfig file"
    exit 1
}

# Check for help argument
if [[ "$1" == "-h" || "$1" == "--help" ]]; then
    show_help
fi

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
    echo "Error: kubectl is not installed. Please install it first."
    show_help
fi

# Check if openssl is installed
if ! command -v openssl &> /dev/null; then
    echo "Error: openssl is not installed. Please install it first."
    show_help
fi

# Set default kubeconfig path or use provided one
KUBECONFIG="${1:-$HOME/.kube/config}"

# Verify kubeconfig file exists
if [ ! -f "$KUBECONFIG" ]; then
    echo "Error: Kubeconfig file not found at $KUBECONFIG"
    show_help
fi

echo "Checking certificate expiry dates in kubeconfig: $KUBECONFIG"
echo "--------------------------------------------------"

# Function to decode and check certificate
check_cert() {
    local name="$1"
    local cert_data="$2"
    
    # Decode base64 and get expiry date
    expiry=$(echo "$cert_data" | base64 -d | openssl x509 -enddate -noout 2>/dev/null | cut -d= -f2)
    
    if [ -z "$expiry" ]; then
        echo "  - $name: Could not parse certificate"
    else
        # Convert expiry date to timestamp and compare with current date
        expiry_epoch=$(date -d "$expiry" +%s 2>/dev/null)
        if [ -z "$expiry_epoch" ]; then
            echo "  - $name: Expires on $expiry (could not calculate remaining days)"
            return
        fi
        
        current_epoch=$(date +%s)
        days_left=$(( (expiry_epoch - current_epoch) / 86400 ))
        
        if [ "$days_left" -lt 0 ]; then
            echo "  - $name: EXPIRED on $expiry"
        else
            echo "  - $name: Expires on $expiry (in $days_left days)"
        fi
    fi
}

# Main execution
{
    # Extract and check client certificates
    echo "[Client Certificates]"
    users=$(kubectl config view --raw -o jsonpath='{.users[*].name}')
    for user in $users; do
        cert_data=$(kubectl config view --raw -o jsonpath="{.users[?(@.name == \"$user\")].user.client-certificate-data}")
        if [ -n "$cert_data" ]; then
            check_cert "User $user" "$cert_data"
        fi
    done

    # Extract and check cluster CA certificates
    echo -e "\n[Cluster CA Certificates]"
    clustters=$(kubectl config view --raw -o jsonpath='{.clusters[*].name}')
    for cluster in $clustters; do
        cert_data=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name == \"$cluster\")].cluster.certificate-authority-data}")
        if [ -n "$cert_data" ]; then
            check_cert "Cluster $cluster" "$cert_data"
        fi
    done

    echo -e "\nDone."
} || {
    echo "An error occurred while processing the kubeconfig file."
    show_help
}