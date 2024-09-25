Name:     topograph
Version:  __VERSION__
Release:  __RELEASE__
Summary:  cluster topology generator

Source0:  topograph
Source1:  topograph-config.yaml
Source2:  configure-ssl.sh
Source3:  create-topology-update-script.sh
Source4:  topograph.service

License:   Proprietary
BuildArch: __ARCH__

%description
discover and generate cluster topology configuration.

%prep

%build

%install
install -D -m 0755 %{SOURCE0} %{buildroot}/usr/local/bin/topograph
install -D -m 0644 %{SOURCE1} %{buildroot}/etc/topograph/topograph-config.yaml
install -D -m 0755 %{SOURCE2} %{buildroot}/etc/topograph/configure-ssl.sh
install -D -m 0755 %{SOURCE3} %{buildroot}/etc/topograph/create-topology-update-script.sh
install -D -m 0644 %{SOURCE4} %{buildroot}/lib/systemd/system/topograph.service

%post
if [ $1 -eq 1 ]; then
    /etc/topograph/configure-ssl.sh
    echo "Package installed"
else
    echo "Package updated"
fi

%files
/usr/local/bin/topograph
/lib/systemd/system/topograph.service
/etc/topograph/topograph-config.yaml
/etc/topograph/configure-ssl.sh
/etc/topograph/create-topology-update-script.sh

%changelog
