#/bin/bash

set -xe

tmux new-session -s nsm -n nsm -d
tmux new-window -t nsm:1 -n nsmd
tmux send-keys -t nsm:1 '$GOPATH/bin/nsmd' C-m
tmux new-window -t nsm:2 -n nse
tmux send-keys -t nsm:2 'NSM_DEVICE_PLUGIN=false $GOPATH/bin/icmp-responder-nse' C-m
tmux -2 attach-session -t nsm

set +xe
