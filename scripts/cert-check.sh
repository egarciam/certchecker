#!/bin/bash
# set -x
# Script to check cert-manager certificate validity in a namespace
# Dependencies: kubectl, jq, GNU date

# --- Configuration ---
# Warn if a certificate is valid but expires within this many days
WARNING_DAYS=30
# --- End Configuration ---

# Function to display usage
usage() {
  echo "Usage: $0 <namespace>"
  echo "Checks the validity of all cert-manager certificates in the specified Kubernetes namespace."
  echo "Requires kubectl, jq, and GNU date to be installed."
  exit 1
}

# Check if namespace argument is provided
if [ -z "$1" ]; then
  usage
fi

NAMESPACE="$1"
# Current date information (as of script execution - Friday, May 9, 2025)
# The script dynamically gets the current date, this is just for context
CURRENT_DATE_EPOCH=$(date +%s)
WARNING_SECONDS=$((WARNING_DAYS * 24 * 60 * 60))

echo "######################################################################"
echo "# Checking cert-manager certificate validity in namespace: $NAMESPACE #"
echo "# Current Date: $(date)                                            #"
echo "# Warning if expires in: $WARNING_DAYS days                               #"
echo "######################################################################"
echo

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo "Error: jq is not installed. Please install jq to run this script."
    exit 1
fi

# Check if kubectl is installed and configured
if ! command -v kubectl &> /dev/null; then
    echo "Error: kubectl is not installed or not in PATH."
    exit 1
fi
if ! kubectl version --client &> /dev/null; then
    echo "Error: kubectl seems not configured correctly or cannot connect to a cluster."
    exit 1
fi


# Get all certificate names in the namespace
# Using a temporary file for certificate names to handle names with spaces if any (though rare for k8s names)
CERT_NAMES_TEMP_FILE=$(mktemp)
if ! kubectl get certificates -n "$NAMESPACE" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' > "$CERT_NAMES_TEMP_FILE"; then
    echo "Error: Failed to retrieve certificates from namespace '$NAMESPACE'."
    echo "Please check if the namespace exists and you have permissions."
    rm "$CERT_NAMES_TEMP_FILE"
    exit 1
fi

if [ ! -s "$CERT_NAMES_TEMP_FILE" ]; then # Check if file is empty
  echo "No cert-manager Certificate resources found in namespace '$NAMESPACE'."
  rm "$CERT_NAMES_TEMP_FILE"
  exit 0
fi

# Loop through each certificate name from the temp file
while IFS= read -r CERT_NAME; do
  if [ -z "$CERT_NAME" ]; then # Skip empty lines if any
      continue
  fi
  echo "--- Certificate: $CERT_NAME ---"

  # Get certificate status using kubectl for easier parsing of specific fields.
  # This is generally more robust for scripting than parsing cmctl's human-readable output.
  CERT_STATUS_JSON=$(kubectl get certificate "$CERT_NAME" -n "$NAMESPACE" -o json)

  if [ -z "$CERT_STATUS_JSON" ]; then
    echo "  Status: Error retrieving details for certificate $CERT_NAME."
    echo "--------------------------------------"
    continue
  fi

  # Extract NotAfter and Ready condition using jq
  NOT_AFTER=$(echo "$CERT_STATUS_JSON" | jq -r '.status.notAfter // "N/A"')
  IS_READY=$(echo "$CERT_STATUS_JSON" | jq -r 'try (.status.conditions[] | select(.type=="Ready") | .status) // "Unknown"')
  READY_REASON=$(echo "$CERT_STATUS_JSON" | jq -r 'try (.status.conditions[] | select(.type=="Ready") | .reason) // "N/A"')
  READY_MESSAGE=$(echo "$CERT_STATUS_JSON" | jq -r 'try (.status.conditions[] | select(.type=="Ready") | .message) // "N/A"')

  echo "  Ready Status: $IS_READY"
  if [ "$IS_READY" != "Unknown" ]; then
    echo "    Reason:   $READY_REASON"
    echo "    Message:  \"$READY_MESSAGE\""
  fi
  echo "  Expires On:   $NOT_AFTER"

  if [ "$NOT_AFTER" == "N/A" ]; then
    echo "  Validity:     Could not determine expiration date."
  else
    # Attempt to parse the NotAfter date
    # This uses GNU date syntax. For macOS, you might need `gdate` or adjust the format string.
    NOT_AFTER_EPOCH=$(date -d "$NOT_AFTER" +%s 2>/dev/null)

    if [ -z "$NOT_AFTER_EPOCH" ]; then
        echo "  Validity:     Error parsing expiration date format ($NOT_AFTER). Ensure GNU date is used or adjust script."
    elif [ "$IS_READY" != "True" ]; then
        echo "  Validity:     NOT READY (Certificate is not in a Ready state)"
        echo "                Check 'cmctl status certificate $CERT_NAME -n $NAMESPACE' or 'kubectl describe certificate $CERT_NAME -n $NAMESPACE' for more details."
    elif [ "$NOT_AFTER_EPOCH" -lt "$CURRENT_DATE_EPOCH" ]; then
        EXPIRED_ON_FORMATTED=$(date -d "@$NOT_AFTER_EPOCH" +"%Y-%m-%d %H:%M:%S %Z")
        echo "  Validity:     EXPIRED (Expired on $EXPIRED_ON_FORMATTED)"
    else
        SECONDS_TO_EXPIRY=$((NOT_AFTER_EPOCH - CURRENT_DATE_EPOCH))
        DAYS_TO_EXPIRY=$((SECONDS_TO_EXPIRY / 86400)) # 86400 seconds in a day

        if [ "$SECONDS_TO_EXPIRY" -lt "$WARNING_SECONDS" ]; then
            echo "  Validity:     VALID (Expires in $DAYS_TO_EXPIRY days - WARNING: Expires soon!)"
        else
            echo "  Validity:     VALID (Expires in $DAYS_TO_EXPIRY days)"
        fi
    fi
  fi
  echo "--------------------------------------"

done < "$CERT_NAMES_TEMP_FILE"

# Clean up temp file
rm "$CERT_NAMES_TEMP_FILE"

echo
echo "######################################################################"
echo "# Finished checking certificates in namespace: $NAMESPACE           #"
echo "######################################################################"