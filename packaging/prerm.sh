#!/bin/sh

SERVICE_NAME="traefik-watch.service"

for user in $(getent passwd | awk -F: '{print $1}') ; do
	if [ $user = "root" ] ; then continue ; fi
	enabled=$(systemctl --user --machine=${user}@ is-enabled ${SERVICE_NAME})
	if [ $enabled = "enabled" ] ; then
		echo "Disabling user ${user} ${SERVICE_NAME}"
		systemctl --user --machine=${user}@ disable ${SERVICE_NAME}
	fi
	active=$(systemctl --user --machine=${user}@ is-active ${SERVICE_NAME})
	if [ $active = "active" ] ; then
		echo "Stopping user ${user} ${SERVICE_NAME}"
		systemctl --user --machine=${user}@ stop ${SERVICE_NAME}
	fi
done
