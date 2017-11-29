%define debug_package %{nil}
%global __strip /bin/true
%global _dwz_low_mem_die_limit 0

%define go_version 1.9.2

Summary: iostat monitoring for Zabbix
Name: zabbix-iostat-mon
Version: 1.0
Release: 1
License: WTFPL
Group: System Environment/Daemons
Source0: %{name}.go
Source1: %{name}.cron
Source3: gopkgs.tar.gz
ExclusiveArch: x86_64
Requires: zabbix-sender >= 2.2, sysstat >= 9.1.2
BuildRequires: golang

%description
iostat monitoring for Zabbix

%prep
tar -C ${RPM_BUILD_DIR} -xzf %{SOURCE3}
mkdir -p ${RPM_BUILD_DIR}/go/src/%{name}
cp -f %{SOURCE0} ${RPM_BUILD_DIR}/go/src/%{name}

%build
export GOARCH="amd64"
export GOROOT="/usr/local/go"
export GOTOOLDIR="/usr/local/go/pkg/tool/linux_amd64"
export GOPATH="${RPM_BUILD_DIR}/go"
export PATH="$PATH:$GOROOT/bin:$GOPATH/bin"

go build -a -ldflags "-X main.Version=%{version}.%{release} -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \n')" -v -x %{name}

%install
install -d %{buildroot}%{_bindir}
cp -f ${RPM_BUILD_DIR}/%{name} %{buildroot}%{_bindir}/%{name}
install -d %{buildroot}/var/log/%{name}
install -d %{buildroot}/etc/cron.d
cp -f %{SOURCE1}  %{buildroot}/etc/cron.d/%{name}

%clean
rm -rf %{buildroot}

%files
%defattr(-,root,root,-)
%{_bindir}/%{name}
%dir /var/log/%{name}
/etc/cron.d/%{name}

%changelog
* Thu Nov 13 2017 Alex Emergy <alex.emergy@gmail.com> - 1.0
- Initial RPM release for EL7.

