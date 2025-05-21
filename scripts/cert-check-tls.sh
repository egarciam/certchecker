#!/bin/bash

# Script to check cert-manager certificate validity and unmanaged TLS secrets.
# Can operate on a single namespace or all accessible namespaces.
# Allows specifying a KUBECONFIG file.
# Dependencies: kubectl, jq, openssl, GNU date
# set -x
# --- Configuration ---
# Warn if a certificate is valid but expires within this many days
WARNING_DAYS=30
# --- End Configuration ---

# Function to display usage
usage() {
  echo "Usage: $0 [OPTIONS] <namespace>"
  echo "   or: $0 [OPTIONS] <-A | --all-namespaces>"
  echo
  echo "Checks the validity of TLS certificates in Kubernetes."
  echo
  echo "Arguments:"
  echo "  <namespace>              Check only the specified namespace (required if -A is not used)."
  echo "  -A, --all-namespaces   Check all accessible namespaces."
  echo
  echo "Options:"
  echo "  -k, --kubeconfig <path>  Path to the kubeconfig file to use."
  echo "  -h, --help               Display this help message and exit."
  echo
  echo "Checks performed:"
  echo "  1. cert-manager Certificate resources (status, expiry)."
  echo "  2. kubernetes.io/tls Secrets not actively managed by a cert-manager Certificate resource (expiry from cert data)."
  echo
  echo "Requires kubectl, jq, openssl, and GNU date to be installed."
  exit 0
}

# --- Global constants ---
CURRENT_DATE_EPOCH=$(date +%s)
WARNING_SECONDS=$((WARNING_DAYS * 24 * 60 * 60))

# --- Dependency Checks (run once) ---
check_dependencies() {
  for cmd in kubectl jq openssl date; do
    if ! command -v $cmd &> /dev/null; then
      echo "Error: Required command '$cmd' is not installed or not in PATH."
      exit 1
    fi
  done
  if ! date -d "now" > /dev/null 2>&1; then
      echo "Error: 'date -d' command not behaving as expected (GNU date feature)." >&2
      echo "If on macOS, you might need to install 'coreutils' (brew install coreutils) and use 'gdate' (and potentially modify script)." >&2
      exit 1
  fi
  # This kubectl command will use the KUBECONFIG if it was set
  if ! kubectl version --client &> /dev/null; then
      echo "Error: kubectl seems not configured correctly or cannot connect to a cluster (check KUBECONFIG or default setup)." >&2
      exit 1
  fi
}

# --- Core logic to check a single namespace ---
check_namespace() {
  local NAMESPACE="$1"
  # Local associative array to store names of secrets managed by cert-manager Certificates
  local -A MANAGED_SECRET_NAMES_MAP

  echo
  echo "=================================================================================="
  echo "# Processing Namespace: $NAMESPACE                                               #"
  echo "=================================================================================="
  echo
  echo "----------------------------------------------------------------------------------"
  echo "- Section 1: Checking cert-manager Certificate Resources in namespace '$NAMESPACE' -"
  echo "----------------------------------------------------------------------------------"

  local CM_CERT_NAMES_TEMP_FILE
  CM_CERT_NAMES_TEMP_FILE=$(mktemp)
  # Get all cert-manager Certificate names in the namespace
  if ! kubectl get certificates -n "$NAMESPACE" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' > "$CM_CERT_NAMES_TEMP_FILE" 2>/dev/null; then
      echo "Notice: Could not retrieve cert-manager Certificates from namespace '$NAMESPACE' (may not exist, no permissions, or CRD not installed)."
  fi

  if [ ! -s "$CM_CERT_NAMES_TEMP_FILE" ] || ! grep -q '[^[:space:]]' "$CM_CERT_NAMES_TEMP_FILE"; then # Check if file is empty or only whitespace
    echo "No cert-manager Certificate resources found in namespace '$NAMESPACE'."
  else
    while IFS= read -r CERT_NAME; do
      if [ -z "$CERT_NAME" ]; then continue; fi # Skip empty lines
      echo
      echo "--- cert-manager Certificate: $CERT_NAME (Namespace: $NAMESPACE) ---"

      local CERT_STATUS_JSON
      CERT_STATUS_JSON=$(kubectl get certificate "$CERT_NAME" -n "$NAMESPACE" -o json 2>/dev/null)

      if [ -z "$CERT_STATUS_JSON" ]; then
        echo "  Status: Error retrieving details for Certificate $CERT_NAME."
        echo "-----------------------------------------------------"
        continue
      fi

      local CM_SECRET_NAME
      CM_SECRET_NAME=$(echo "$CERT_STATUS_JSON" | jq -r '.spec.secretName // "N/A"')
      if [ "$CM_SECRET_NAME" != "N/A" ]; then
        MANAGED_SECRET_NAMES_MAP["$CM_SECRET_NAME"]=1
        echo "  Manages Secret: $CM_SECRET_NAME"
      fi

      local NOT_AFTER IS_READY READY_REASON READY_MESSAGE
      NOT_AFTER=$(echo "$CERT_STATUS_JSON" | jq -r '.status.notAfter // "N/A"')
      IS_READY=$(echo "$CERT_STATUS_JSON" | jq -r 'try (.status.conditions[] | select(.type=="Ready") | .status) // "Unknown"')
      READY_REASON=$(echo "$CERT_STATUS_JSON" | jq -r 'try (.status.conditions[] | select(.type=="Ready") | .reason) // "N/A"')
      READY_MESSAGE=$(echo "$CERT_STATUS_JSON" | jq -r 'try (.status.conditions[] | select(.type=="Ready") | .message) // "N/A"')

      echo "  Ready Status: $IS_READY"
      if [ "$IS_READY" != "Unknown" ]; then
        echo "    Reason:   $READY_REASON"
        echo "    Message:  \"$READY_MESSAGE\""
      fi
      echo "  Expires On:   $NOT_AFTER (according to Certificate resource)"

      if [ "$NOT_AFTER" == "N/A" ]; then
        echo "  Validity:     Could not determine expiration date from Certificate resource."
      else
        local NOT_AFTER_EPOCH
        NOT_AFTER_EPOCH=$(date -d "$NOT_AFTER" +%s 2>/dev/null)

        if [ -z "$NOT_AFTER_EPOCH" ]; then
            echo "  Validity:     Error parsing expiration date format ($NOT_AFTER)."
        elif [ "$IS_READY" != "True" ]; then
            echo "  Validity:     NOT READY (Certificate resource is not in a Ready state)"
            echo "                Check 'cmctl status certificate $CERT_NAME -n $NAMESPACE' for details."
        elif [ "$NOT_AFTER_EPOCH" -lt "$CURRENT_DATE_EPOCH" ]; then
            local EXPIRED_ON_FORMATTED
            EXPIRED_ON_FORMATTED=$(date -d "@$NOT_AFTER_EPOCH" +"%Y-%m-%d %H:%M:%S %Z")
            echo "  Validity:     EXPIRED (Expired on $EXPIRED_ON_FORMATTED)"
        else
            local SECONDS_TO_EXPIRY DAYS_TO_EXPIRY
            SECONDS_TO_EXPIRY=$((NOT_AFTER_EPOCH - CURRENT_DATE_EPOCH))
            DAYS_TO_EXPIRY=$((SECONDS_TO_EXPIRY / 86400))

            if [ "$SECONDS_TO_EXPIRY" -lt "$WARNING_SECONDS" ]; then
                echo "  Validity:     VALID (Expires in $DAYS_TO_EXPIRY days - WARNING: Expires soon!)"
            else
                echo "  Validity:     VALID (Expires in $DAYS_TO_EXPIRY days)"
            fi
        fi
      fi
      echo "-----------------------------------------------------"
    done < "$CM_CERT_NAMES_TEMP_FILE"
  fi
  rm "$CM_CERT_NAMES_TEMP_FILE"


  echo
  echo "----------------------------------------------------------------------------------"
  echo "- Section 2: Checking Unmanaged kubernetes.io/tls Secrets in namespace '$NAMESPACE' -"
  echo "- (Secrets of type kubernetes.io/tls not linked to an existing Certificate resource) -"
  echo "----------------------------------------------------------------------------------"

  local TLS_SECRET_NAMES_TEMP_FILE
  TLS_SECRET_NAMES_TEMP_FILE=$(mktemp)
  # Get all TLS secret names in the namespace
  if ! kubectl get secrets -n "$NAMESPACE" --field-selector type=kubernetes.io/tls -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' > "$TLS_SECRET_NAMES_TEMP_FILE" 2>/dev/null; then
      echo "Notice: Could not retrieve TLS Secrets from namespace '$NAMESPACE' (may not exist or no permissions)."
  fi

  if [ ! -s "$TLS_SECRET_NAMES_TEMP_FILE" ] || ! grep -q '[^[:space:]]' "$TLS_SECRET_NAMES_TEMP_FILE"; then
    echo "No kubernetes.io/tls Secrets found in namespace '$NAMESPACE'."
  else
    local SECRETS_CHECKED=0
    local UNMANAGED_SECRETS_FOUND=0
    while IFS= read -r SECRET_NAME; do
      if [ -z "$SECRET_NAME" ]; then continue; fi
      SECRETS_CHECKED=$((SECRETS_CHECKED + 1))

      if [[ -n "${MANAGED_SECRET_NAMES_MAP[$SECRET_NAME]}" ]]; then
        echo
        echo "--- TLS Secret: $SECRET_NAME (Namespace: $NAMESPACE) ---"
        echo "  Status: Managed by cert-manager Certificate (details in Section 1)."
        echo "-----------------------------------------------------"
        continue
      fi

      UNMANAGED_SECRETS_FOUND=$((UNMANAGED_SECRETS_FOUND + 1))
      echo
      echo "--- Unmanaged TLS Secret: $SECRET_NAME (Namespace: $NAMESPACE) ---"

      local SECRET_JSON
      SECRET_JSON=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o json 2>/dev/null)
      if [ -z "$SECRET_JSON" ]; then
        echo "  Status: Error retrieving details for Secret $SECRET_NAME."
        echo "-----------------------------------------------------"
        continue
      fi

      local CERT_DATA_BASE64
      CERT_DATA_BASE64=$(echo "$SECRET_JSON" | jq -r '.data."tls.crt" // ""')
      if [ -z "$CERT_DATA_BASE64" ]; then
        echo "  TLS Data: Error - 'tls.crt' field is missing or empty in Secret $SECRET_NAME."
        echo "-----------------------------------------------------"
        continue
      fi

      local CERT_DATA_DECODED
      CERT_DATA_DECODED=$(echo "$CERT_DATA_BASE64" | base64 --decode 2>/dev/null)
      if [ -z "$CERT_DATA_DECODED" ]; then
          echo "  TLS Data: Error - Failed to base64 decode 'tls.crt' from Secret $SECRET_NAME."
          echo "-----------------------------------------------------"
          continue
      fi

      if ! echo "$CERT_DATA_DECODED" | openssl x509 -noout > /dev/null 2>&1; then
          echo "  TLS Data: Error - 'tls.crt' in Secret $SECRET_NAME is not a valid PEM certificate."
          echo "-----------------------------------------------------"
          continue
      fi

      local SUBJECT_LINE ISSUER_LINE START_DATE_STR_OPENSSL END_DATE_STR_OPENSSL
      SUBJECT_LINE=$(echo "$CERT_DATA_DECODED" | openssl x509 -noout -subject 2>/dev/null | sed 's/subject= *//')
      ISSUER_LINE=$(echo "$CERT_DATA_DECODED" | openssl x509 -noout -issuer 2>/dev/null | sed 's/issuer= *//')
      START_DATE_STR_OPENSSL=$(echo "$CERT_DATA_DECODED" | openssl x509 -noout -startdate 2>/dev/null | cut -d= -f2-)
      END_DATE_STR_OPENSSL=$(echo "$CERT_DATA_DECODED" | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2-)

      echo "  Subject:    $SUBJECT_LINE"
      echo "  Issuer:     $ISSUER_LINE"
      echo "  Valid From: $START_DATE_STR_OPENSSL (according to cert data)"
      echo "  Expires On: $END_DATE_STR_OPENSSL (according to cert data)"

      local NOT_BEFORE_EPOCH NOT_AFTER_EPOCH
      NOT_BEFORE_EPOCH=$(date -d "$START_DATE_STR_OPENSSL" +%s 2>/dev/null)
      NOT_AFTER_EPOCH=$(date -d "$END_DATE_STR_OPENSSL" +%s 2>/dev/null)

      if [ -z "$NOT_BEFORE_EPOCH" ] || [ -z "$NOT_AFTER_EPOCH" ]; then
          echo "  Validity:     Error parsing certificate dates from OpenSSL output."
      elif [ "$CURRENT_DATE_EPOCH" -lt "$NOT_BEFORE_EPOCH" ]; then
          echo "  Validity:     NOT YET VALID (Starts on $(date -d "@$NOT_BEFORE_EPOCH" +"%Y-%m-%d %H:%M:%S %Z"))"
      elif [ "$NOT_AFTER_EPOCH" -lt "$CURRENT_DATE_EPOCH" ]; then
          echo "  Validity:     EXPIRED (Expired on $(date -d "@$NOT_AFTER_EPOCH" +"%Y-%m-%d %H:%M:%S %Z"))"
      else
          local SECONDS_TO_EXPIRY DAYS_TO_EXPIRY
          SECONDS_TO_EXPIRY=$((NOT_AFTER_EPOCH - CURRENT_DATE_EPOCH))
          DAYS_TO_EXPIRY=$((SECONDS_TO_EXPIRY / 86400))

          if [ "$SECONDS_TO_EXPIRY" -lt "$WARNING_SECONDS" ]; then
              echo "  Validity:     VALID (Expires in $DAYS_TO_EXPIRY days - WARNING: Expires soon!)"
          else
              echo "  Validity:     VALID (Expires in $DAYS_TO_EXPIRY days)"
          fi
      fi
      echo "-----------------------------------------------------"
    done < "$TLS_SECRET_NAMES_TEMP_FILE"

    if [ "$SECRETS_CHECKED" -eq 0 ] && [ "$UNMANAGED_SECRETS_FOUND" -eq 0 ]; then
      : # No kubernetes.io/tls Secrets found, covered by initial temp file check
    elif [ "$UNMANAGED_SECRETS_FOUND" -eq 0 ] && [ "$SECRETS_CHECKED" -gt 0 ]; then
      echo "All found kubernetes.io/tls Secrets in namespace '$NAMESPACE' appear to be managed by cert-manager Certificate resources."
    fi
  fi
  rm "$TLS_SECRET_NAMES_TEMP_FILE"
}

# --- Argument Parsing ---
ARG_KUBECONFIG_PATH=""
ARG_ALL_NAMESPACES_MODE=false
ARG_TARGET_NAMESPACE=""
POSITIONAL_ARGS=()

if [ $# -eq 0 ]; then
    usage # No arguments provided
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    -k|--kubeconfig)
      if [ -n "$2" ] && [[ "$2" != -* ]]; then
        ARG_KUBECONFIG_PATH="$2"
        shift # past argument
        shift # past value
      else
        echo "Error: --kubeconfig requires a non-empty argument." >&2
        usage # exit
      fi
      ;;
    -A|--all-namespaces)
      ARG_ALL_NAMESPACES_MODE=true
      shift # past argument
      ;;
    -h|--help)
      usage # exit
      ;;
    -*) # Unknown option
      echo "Error: Unknown option $1" >&2
      usage # exit
      ;;
    *) # Positional argument
      POSITIONAL_ARGS+=("$1")
      shift # past argument
      ;;
  esac
done

# Process positional arguments
if [ "$ARG_ALL_NAMESPACES_MODE" = true ]; then
    if [ "${#POSITIONAL_ARGS[@]}" -gt 0 ]; then
        echo "Warning: Positional arguments ('${POSITIONAL_ARGS[*]}') ignored when -A/--all-namespaces is used." >&2
    fi
    # ARG_TARGET_NAMESPACE remains empty
elif [ "${#POSITIONAL_ARGS[@]}" -eq 1 ]; then
    ARG_TARGET_NAMESPACE="${POSITIONAL_ARGS[0]}"
elif [ "${#POSITIONAL_ARGS[@]}" -gt 1 ]; then
    echo "Error: Too many positional arguments. Expected one namespace or -A/--all-namespaces." >&2
    usage # exit
else # No positional arguments and not in -A mode
    echo "Error: You must specify a namespace or use -A/--all-namespaces." >&2
    usage # exit
fi


# --- Main Script Logic ---

# Set KUBECONFIG if provided by argument
if [ -n "$ARG_KUBECONFIG_PATH" ]; then
  if [ -f "$ARG_KUBECONFIG_PATH" ]; then
    export KUBECONFIG="$ARG_KUBECONFIG_PATH"
    echo "INFO: Using KUBECONFIG override: $KUBECONFIG"
  else
    echo "Error: Kubeconfig file not found at '$ARG_KUBECONFIG_PATH'" >&2
    exit 1
  fi
# Else, kubectl will use default KUBECONFIG (e.g., ~/.kube/config or in-cluster)
fi

# Perform initial dependency checks (kubectl version check will now use the overridden KUBECONFIG if set)
check_dependencies

echo
echo "##################################################################################"
echo "# Starting Certificate Validity Check                                            #"
echo "# Current Date: $(date +"%Y-%m-%d %H:%M:%S %Z")                                   #"
echo "# Warning if expires in: $WARNING_DAYS days                                        #"
if [ -n "$KUBECONFIG" ]; then
echo "# Using KUBECONFIG: $KUBECONFIG                                                #"
fi
echo "##################################################################################"

if [ "$ARG_ALL_NAMESPACES_MODE" = true ]; then
  echo
  echo "Mode: Checking ALL accessible namespaces."
  ALL_NS_LIST_TEMP_FILE=$(mktemp)
  # Attempt to get namespaces, redirect stderr for cleaner output if it fails here
  if ! kubectl get namespaces -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' > "$ALL_NS_LIST_TEMP_FILE" 2>/dev/null; then
      echo "Error: Could not retrieve list of namespaces. Check permissions or cluster connectivity. Exiting."
      rm "$ALL_NS_LIST_TEMP_FILE"
      exit 1
  fi
  cat $ALL_NS_LIST_TEMP_FILE
  if [ ! -s "$ALL_NS_LIST_TEMP_FILE" ] || ! grep -q '[^[:space:]]' "$ALL_NS_LIST_TEMP_FILE"; then
      echo "Error: No namespaces found or accessible. Exiting."
      rm "$ALL_NS_LIST_TEMP_FILE"
      exit 1
  fi

  while IFS= read -r NS_NAME; do
    if [ -z "$NS_NAME" ]; then continue; fi
    check_namespace "$NS_NAME"
  done < "$ALL_NS_LIST_TEMP_FILE"
  rm "$ALL_NS_LIST_TEMP_FILE"
else
  # ARG_TARGET_NAMESPACE should be set if not in ARG_ALL_NAMESPACES_MODE due to prior checks
  echo
  echo "Mode: Checking single namespace: $ARG_TARGET_NAMESPACE"
  check_namespace "$ARG_TARGET_NAMESPACE"
fi

echo
echo "##################################################################################"
echo "# Certificate Validity Check Completed                                           #"
echo "##################################################################################"