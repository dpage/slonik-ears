# Slonik Ears Ansible Playbooks

These playbooks build a relay server and any number of room machines
from fresh Debian 13 installs, generating the tokens on the way. The
[Deploying with Ansible](../docs/ansible.md) page describes them in full.

Copy the example inventory, edit it, and run everything from this
directory:

```bash
cp -R inventory/example inventory/myevent
ansible-playbook -i inventory/myevent/hosts.yml site.yml --ask-become-pass
```

Git ignores every inventory except the example, and the `credentials/`
directory where the generated tokens are kept.
