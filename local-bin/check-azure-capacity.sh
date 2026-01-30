#!/usr/bin/env bash

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Other variables
REGIONS_FILE="regions.txt"

# Change to the known temporary directory
TMP_DIR="/tmp/azure-capacity-check"

# Function to display usage
function usage() {
    echo "Usage: $0 <subcommand> [options]"
    echo ""
    echo "Subcommands:"
    echo "  print        Check Azure VM capacity in specified regions and VM sizes"
    echo "  try          Attempt to scale a specific VM size in a region"
    echo "  who-uses     Find who is using a specific VM type in resource groups"
    echo "  help, -h     Show this help message"
    echo ""
    echo "Options for 'print':"
    echo "  --vm-sizes   One or more VM sizes to check (e.g., Standard_D2s_v3)"
    echo "  --regions    Optional: One or more regions to check. If not specified, checks all regions"
    echo ""
    echo "Options for 'try':"
    echo "  --vm-size    The VM size to scale (e.g., Standard_D2s_v3)"
    echo "  --region     The region to scale in (e.g., eastus)"
    echo "  --scale      The scale value to attempt (e.g., 10)"
    echo ""
    echo "Options for 'who-uses':"
    echo "  --vm-size    The VM size to search for (e.g., Standard_ND96isr_H100_v5)"
    echo "  --refresh    Force re-download of VM and VMSS data (optional)"
    echo ""
    echo "Examples:"
    echo "  $0 print     --vm-sizes Standard_D2s_v3 Standard_D4s_v3 --regions eastus westus"
    echo "  $0 try       --vm-size Standard_D2s_v3 --region eastus --scale 10"
    echo "  $0 who-uses  --vm-size Standard_ND96isr_H100_v5"
    echo "  $0 who-uses  --vm-size Standard_ND96isr_H100_v5 --refresh"
    exit 1
}

# Function to check if Azure CLI is installed
function check_azure_cli() {
    if ! command -v az &>/dev/null; then
        echo -e "${RED}Error: Azure CLI is not installed${NC}"
        echo "Please install Azure CLI: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli"
        exit 1
    fi
}

# Function to check if user is logged in to Azure
function check_azure_login() {
    if ! az account show &>/dev/null; then
        echo -e "${RED}Error: Not logged in to Azure${NC}"
        echo "Please run 'az login' first"
        exit 1
    fi
}

# Function to draw table
function draw_table_header() {
    local max_vm_size_length=0
    local max_region_length=0

    # Find the longest VM size
    for vm_size in "${vm_sizes[@]}"; do
        if [ ${#vm_size} -gt $max_vm_size_length ]; then
            max_vm_size_length=${#vm_size}
        fi
    done

    # Find the longest region
    for region in "${regions[@]}"; do
        if [ ${#region} -gt $max_region_length ]; then
            max_region_length=${#region}
        fi
    done

    # Define column widths
    VM_SIZE_WIDTH=$((max_vm_size_length > 8 ? max_vm_size_length : 8))
    REGION_WIDTH=$((max_region_length > 8 ? max_region_length : 8))
    AVAILABLE_WIDTH=9
    RESTRICTIONS_WIDTH=12

    # Print the header
    printf "\n${BOLD}%-${VM_SIZE_WIDTH}s | %-${REGION_WIDTH}s | %-${AVAILABLE_WIDTH}s | %-${RESTRICTIONS_WIDTH}s${NC}\n" "VM Size" "Region" "Available" "Restrictions"
    printf "%-${VM_SIZE_WIDTH}s-|-%-${REGION_WIDTH}s-|-%-${AVAILABLE_WIDTH}s-|-%-${RESTRICTIONS_WIDTH}s\n" \
        "$(printf -- '-%.0s' $(seq 1 $VM_SIZE_WIDTH))" \
        "$(printf -- '-%.0s' $(seq 1 $REGION_WIDTH))" \
        "$(printf -- '-%.0s' $(seq 1 $AVAILABLE_WIDTH))" \
        "$(printf -- '-%.0s' $(seq 1 $RESTRICTIONS_WIDTH))"
}

# Function to print table rows dynamically
function print_table_row() {
    local vm_size="$1"
    local region="$2"
    local available="$3"
    local restrictions="$4"

    # Strip ANSI escape sequences for alignment
    local available_plain=$(echo -e "$available" | sed 's/\033\[[0-9;]*m//g')

    # Calculate padding for the "Available" column
    local available_padding=$((AVAILABLE_WIDTH - ${#available_plain} - 1))
    local available_formatted="$(echo -e "$available")$(printf '%*s' $available_padding)"

    # Use printf to ensure proper alignment
    printf "%-${VM_SIZE_WIDTH}s | %-${REGION_WIDTH}s | %-${AVAILABLE_WIDTH}s | %-${RESTRICTIONS_WIDTH}s\n" "$vm_size" "$region" "$available_formatted" "$restrictions"
}

function fetch_regions() {
    # Check if regions.txt exists
    if [ -f "$REGIONS_FILE" ]; then
        echo -e "${YELLOW}Using cached regions from $REGIONS_FILE...${NC}"
        regions=($(cat "$REGIONS_FILE"))
    else
        echo -e "${YELLOW}No regions specified, getting all available regions...${NC}"
        while IFS= read -r line; do
            regions+=("$line")
        done < <(az account list-locations --query "[].name" -o tsv)

        # Let's save the regions to a file for future use
        echo "${regions[@]}" >"$REGIONS_FILE"
    fi
}

function print_regions() {
    # Check if VM sizes are provided
    if [ ${#vm_sizes[@]} -eq 0 ]; then
        echo -e "${RED}Error: No VM sizes specified${NC}"
        usage
    fi

    # Main script
    echo -e "${YELLOW}Checking Azure environment...${NC}"
    check_azure_cli
    check_azure_login

    mkdir -p "$TMP_DIR"
    pushd "$TMP_DIR"
    # If no regions specified, get all regions
    if [ ${#regions[@]} -eq 0 ]; then
        fetch_regions
    fi

    echo -e "${YELLOW}Starting capacity check...${NC}"
    draw_table_header

    # Check each combination of region and VM size
    for region in "${regions[@]}"; do
        # Check if the region file exists before fetching
        if [ -f "${region}.json" ]; then
            result=$(cat "${region}.json")
        else
            result=$(az vm list-skus --location "${region}")
            echo "${result}" >"${region}.json"
        fi

        for vm_size in "${vm_sizes[@]}"; do
            if [ "$result" != "null" ]; then
                # Check for restrictions
                restrictions=$(echo "$result" | jq -r --arg vm_size "$vm_size" '.[] | select(.resourceType == "virtualMachines" and .name == $vm_size) | .restrictions[0].reasonCode')
                if [ -z "$restrictions" ] || [ "$restrictions" == "null" ]; then
                    # No restrictions found - capacity is available
                    print_table_row "$vm_size" "$region" "${GREEN}Yes${NC}" "None"
                else
                    # Restrictions found - format them
                    formatted_restrictions=$(echo "$restrictions" | tr '\n' ',' | sed 's/,$//')
                    print_table_row "$vm_size" "$region" "${RED}No${NC}" "$formatted_restrictions"
                fi
            else
                # SKU not found in region
                print_table_row "$vm_size" "$region" "${RED}No${NC}" "SKU not available"
            fi
        done
    done

    echo -e "\n${YELLOW}Capacity check complete!${NC}"
    popd
}

# This function takes vm size, region and scale as arguments
# and attempts to scale the VM size in the specified region.
function try_vmss() {
    local vm_size="$1"
    local region="$2"
    local scale="$3"

    # Main script for 'try' subcommand
    echo -e "${YELLOW}Checking Azure environment...${NC}"
    check_azure_cli
    check_azure_login

    echo -e "${YELLOW}Trying to scale VM size $vm_size in region $region to $scale...${NC}"

    rg_name="suraj-trial-$RANDOM"
    az group create --name "$rg_name" --location "$region"
    trap "az group delete --name $rg_name --yes" EXIT

    scale_set_name="trial-vmss-$RANDOM"
    az vmss create \
        --name "$scale_set_name" \
        --resource-group "$rg_name" \
        --instance-count "$scale" \
        --vm-sku "$vm_size" \
        --location "$region" \
        --image Ubuntu2404

    echo -e "${GREEN}Scale operation complete!${NC}"
}

# This function finds which resource groups are using a specific VM type
function who_uses() {
    local vm_type="$1"
    local refresh="$2"

    echo -e "${YELLOW}Checking Azure environment...${NC}"
    check_azure_cli
    check_azure_login

    echo -e "${YELLOW}Finding resource groups using VM type: $vm_type${NC}"

    # Get current subscription
    SUB=$(az account show --query id --output tsv)
    echo -e "${YELLOW}Subscription: ${SUB}${NC}"

    # Create temporary directory for cache files
    mkdir -p "${TMP_DIR}"

    # Remove cache files if refresh is requested
    if [ "${refresh}" = "true" ]; then
        echo -e "${YELLOW}Refresh requested, removing cached files...${NC}"
        rm -f "${TMP_DIR}/all_vms.json" "${TMP_DIR}/all_vmss.json"
    fi

    # Fetch all VMs and VMSSs
    if [ -f "${TMP_DIR}/all_vms.json" ]; then
        echo -e "${YELLOW}Using cached VMs from ${TMP_DIR}/all_vms.json${NC}"
    else
        echo -e "${YELLOW}Fetching all VMs...${NC}"
        az vm list -d --subscription "${SUB}" -o json > "${TMP_DIR}/all_vms.json"
    fi

    if [ -f "${TMP_DIR}/all_vmss.json" ]; then
        echo -e "${YELLOW}Using cached VMSSs from ${TMP_DIR}/all_vmss.json${NC}"
    else
        echo -e "${YELLOW}Fetching all VMSSs...${NC}"
        az vmss list --subscription "${SUB}" -o json > "${TMP_DIR}/all_vmss.json"
    fi

    echo ""
    echo -e "${BOLD}Results:${NC}"
    echo ""

    # Check VMs
    vm_results=$(cat "${TMP_DIR}/all_vms.json" | jq -r --arg vmtype "${vm_type}" '.[] | select(.hardwareProfile.vmSize == $vmtype) | "\(.name) | \(.resourceGroup) | \(.location)"')

    if [ -n "${vm_results}" ]; then
        echo -e "${BOLD}Virtual Machines using ${vm_type}:${NC}"
        printf "%-40s | %-50s | %-20s\n" "VM Name" "Resource Group" "Location"
        printf "%-40s-|-%-50s-|-%-20s\n" "$(printf -- '-%.0s' $(seq 1 40))" "$(printf -- '-%.0s' $(seq 1 50))" "$(printf -- '-%.0s' $(seq 1 20))"
        echo "${vm_results}" | while IFS='|' read -r name rg location; do
            printf "${GREEN}%-40s${NC} | ${YELLOW}%-50s${NC} | %-20s\n" "$(echo "${name}" | xargs)" "$(echo "${rg}" | xargs)" "$(echo "${location}" | xargs)"
        done
        echo ""
    else
        echo -e "${YELLOW}No VMs found using ${vm_type}${NC}"
        echo ""
    fi

    # Check VMSSs
    vmss_results=$(cat "${TMP_DIR}/all_vmss.json" | jq -r --arg vmtype "${vm_type}" '.[] | select(.sku.name == $vmtype) | "\(.name) | \(.resourceGroup) | \(.location) | \(.sku.capacity)"')

    if [ -n "${vmss_results}" ]; then
        echo -e "${BOLD}Virtual Machine Scale Sets using ${vm_type}:${NC}"
        printf "%-40s | %-50s | %-20s | %-8s\n" "VMSS Name" "Resource Group" "Location" "Capacity"
        printf "%-40s-|-%-50s-|-%-20s-|-%-8s\n" "$(printf -- '-%.0s' $(seq 1 40))" "$(printf -- '-%.0s' $(seq 1 50))" "$(printf -- '-%.0s' $(seq 1 20))" "$(printf -- '-%.0s' $(seq 1 8))"
        echo "${vmss_results}" | while IFS='|' read -r name rg location capacity; do
            printf "${GREEN}%-40s${NC} | ${YELLOW}%-50s${NC} | %-20s | %-8s\n" "$(echo "${name}" | xargs)" "$(echo "${rg}" | xargs)" "$(echo "${location}" | xargs)" "$(echo "${capacity}" | xargs)"
        done
        echo ""

        # Check for AKS-managed resource groups and provide portal links
        echo -e "${BOLD}AKS Cluster Links:${NC}"
        aks_found=false
        while IFS='|' read -r name rg location capacity; do
            rg_trimmed=$(echo "${rg}" | xargs)
            # AKS managed resource groups follow the pattern: MC_{resourceGroup}_{aksClusterName}_{location}
            if [[ "${rg_trimmed}" =~ ^MC_([^_]+)_([^_]+)_.+$ ]]; then
                aks_found=true
                aks_rg="${BASH_REMATCH[1]}"
                aks_name="${BASH_REMATCH[2]}"
                portal_url="https://portal.azure.com/#@/resource/subscriptions/${SUB}/resourceGroups/${aks_rg}/providers/Microsoft.ContainerService/managedClusters/${aks_name}/overview"
                echo -e "  ${GREEN}${aks_name}${NC} (in resource group ${YELLOW}${aks_rg}${NC}):"
                echo -e "    ${portal_url}"
            fi
        done <<< "${vmss_results}"

        if [ "${aks_found}" = false ]; then
            echo -e "  ${YELLOW}No AKS-managed resource groups detected${NC}"
        fi
        echo ""
    else
        echo -e "${YELLOW}No VMSSs found using ${vm_type}${NC}"
        echo ""
    fi

    echo -e "${GREEN}Search complete!${NC}"
}

# Parse command line arguments
subcommand=""
vm_sizes=()
regions=()
vm_size=""
region=""
scale=""
refresh=false

if [[ $# -eq 0 ]]; then
    echo -e "${RED}Error: No subcommand specified${NC}"
    usage
fi

subcommand="$1"
shift

case "$subcommand" in
help | --help | -h)
    usage
    ;;
print)
    while [[ $# -gt 0 ]]; do
        case $1 in
        --vm-sizes)
            shift
            while [[ $# -gt 0 && ! $1 =~ ^-- ]]; do
                vm_sizes+=("$1")
                shift
            done
            ;;
        --regions)
            shift
            while [[ $# -gt 0 && ! $1 =~ ^-- ]]; do
                regions+=("$1")
                shift
            done
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            usage
            ;;
        esac
        [[ $# -gt 0 ]] && shift
    done

    print_regions
    ;;
try)
    while [[ $# -gt 0 ]]; do
        case $1 in
        --vm-size)
            shift
            vm_size="$1"
            shift
            ;;
        --region)
            shift
            region="$1"
            shift
            ;;
        --scale)
            shift
            scale="$1"
            shift
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            usage
            ;;
        esac
    done

    # Check if required arguments are provided
    if [ -z "$vm_size" ] || [ -z "$region" ] || [ -z "$scale" ]; then
        echo -e "${RED}Error: Missing required arguments for 'try' subcommand${NC}"
        usage
    fi

    try_vmss "$vm_size" "$region" "$scale"
    ;;
who-uses)
    while [[ $# -gt 0 ]]; do
        case $1 in
        --vm-size)
            shift
            vm_size="$1"
            shift
            ;;
        --refresh)
            refresh=true
            shift
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            usage
            ;;
        esac
    done

    # Check if required argument is provided
    if [ -z "$vm_size" ]; then
        echo -e "${RED}Error: Missing required argument --vm-size for 'who-uses' subcommand${NC}"
        usage
    fi

    who_uses "$vm_size" "$refresh"
    ;;
*)
    echo -e "${RED}Error: Unknown subcommand $subcommand${NC}"
    usage
    ;;
esac
