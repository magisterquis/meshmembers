#!/bin/sh
#
# docreate.sh
# Create a network in DigitalOcean
# By J. Stuart McMurray
# Created 20200418
# Last Modified 20200418

SSH_KEY=
TAG=meshmembers
IMAGE=ubuntu-19-10-x64

set -e

# Create instances in several regions
for region in nyc1 fra1 blr1 sgp1; do
        echo "Creating instances in $region"
        doctl compute droplet create mesh-${region}-1 mesh-${region}-2 mesh-${region}-3 mesh-${region}-4 mesh-${region}-5 --image IMAGE --region ${region} --size s-1vcpu-1gb --ssh-keys $SSH_KEY --tag-name $TAG --user-data-file ./cloud-init.yaml
done
