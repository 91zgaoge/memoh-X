# coding:utf-8
from volcengine.vms.VmsService import VmsService

if __name__ == '__main__':
    vms_service = VmsService()

    vms_service.set_ak("your ak")
    vms_service.set_sk("your sk")

    form = {
        "NumberPoolNo": "xxx"
    }
    print(vms_service.select_number(form))
