# -*- mode: ruby -*-
# vi: set ft=ruby :

VAGRANTFILE_API_VERSION = "2"

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.box = "ubuntu/vivid64" # 15.04

  config.vm.provider "virtualbox" do |vb|
    vb.customize ["modifyvm", :id, "--memory", "2048"]
    vb.customize ["modifyvm", :id, "--cpus", "2"]
    # https://stefanwrobel.com/how-to-make-vagrant-performance-not-suck
    vb.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
    vb.customize ["modifyvm", :id, "--natdnsproxy1", "on"]
  end

  # allows calling docker from host machine if networking properly set up
  # see https://github.com/peter-edge/dotfiles/tree/master/bin/setup_docker_http.sh
  # note that the above script should not be used if your vagrant box is somehow insecure
  # see https://github.com/peter-edge/dotfiles/tree/master/bash_aliases_docker up until
  # the 'alias docker' command for how to set this all up with a mac (after 'brew install docker')
  config.vm.network "private_network", ip: "192.168.10.11"
  # allows volumes from host machine if calling docker from host machine
  config.vm.synced_folder ENV['HOME'], ENV['HOME']

  config.vm.provision "shell", path: "etc/initdev/init.sh", args: "vagrant"
end
