# OBS Teleport

An [OBS Studio] plugin for an open [NDI]-like replacement. Pretty simple, straight forward. No NDI compatibility in any form.

Just as an alternative option for stream setups with multiple machines wanting to transmit some OBS Studio scenes to the main streaming machine in LAN.

![](img/obs-teleport.png)

[OBS Studio]: https://obsproject.com
[NDI]: https://ndi.tv/


## Installation

Please refer to the OBS Studio documentation on how and where to install plugins. There are too many platforms and installation options available as the scope of this project could explain and maintain.

Check here for a starting point:

https://obsproject.com/forum/resources/obs-and-obs-studio-install-plugins-windows.421/

Binaries can be grabbed from the [Releases] section.

[Releases]: https://github.com/fzwoch/obs-teleport/releases


## Setup Sender

Go to `Tools → Teleport`.

![](img/teleport-tools.png)

Check `Teleport Enabled`.

![](img/teleport-output.png)


## Setup Sender as Audio/Video Filter

Click `<Source> Right click → Filters`.

![](img/teleport-properties.png)

Click `+ → Teleport`.

![](img/teleport-filter.png)


## Setup Receiver

In your Scene do `Sources → Add → Teleport`.

![](img/teleport-add.png)

Select a detected stream from the `Teleport` drop down.

![](img/teleport-source.png)
