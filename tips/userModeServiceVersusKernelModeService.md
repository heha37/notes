# User-mode Service Versus Kernel-mode Service

[What would you write kernel module for?](https://stackoverflow.com/questions/26256605/what-would-you-write-kernel-module-for)

A user-mode service or driver has the advantages of:

* Usually easier and quicker to implement (no need to build and install the kernel). Most C programmers learn (only) with the C runtime library, so the kernel environment instead of user-mode could be a learning experience.

* Easier to control proprietary source code. May be exempt from the GNU GPL.

* The restricted privileges are less likely to inadvertently take down the system or create a security hole.

A kernel-mode service or driver has the advantages of:

Availability to more than one program in the system without messy exclusion locks.

* Device accessibility can be controlled by file permissions.

* Status of the device or service is continuous and available as long as the system is running. A user-mode driver would have to reset the device into a known quiescent state everytime the program started.

* More consistent/accurate timers and reduced event latency.