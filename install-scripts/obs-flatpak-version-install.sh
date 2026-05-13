# This install script is specficialyl for the Flatpak install version of OBS Studio
# Main install location
#~/.var/app/com.obsproject.Studio/config/obs-studio
#!/bin/sh

echo "Installing Teleport into ~/.var/app/com.obsproject.Studio/config/obs-studio/plugins/obs-teleport/bin/64bit/obs-teleport.so"
rm -rf ~/.var/app/com.obsproject.Studio/config/obs-studio/plugins/obs-teleport
mkdir -p ~/.var/app/com.obsproject.Studio/config/obs-studio/plugins/obs-teleport/bin/64bit
cp $(dirname $0)/obs-teleport.so ~/.var/app/com.obsproject.Studio/config/obs-studio/plugins/obs-teleport/bin/64bit/obs-teleport.so
echo "Successfully installed OBS Teleport Flatpak installation version"