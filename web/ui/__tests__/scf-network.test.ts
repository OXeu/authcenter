import { execFileSync } from 'node:child_process'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vite-plus/test'

const harness = `
import contextlib
import io
import json
import os
import runpy
import sys
import types

scenario = json.loads(sys.argv[1])
modules = {}
for name in ('tencentcloud', 'tencentcloud.common', 'tencentcloud.common.credential',
             'tencentcloud.common.exception', 'tencentcloud.common.exception.tencent_cloud_sdk_exception',
             'tencentcloud.scf', 'tencentcloud.scf.v20180416',
             'tencentcloud.scf.v20180416.models', 'tencentcloud.scf.v20180416.scf_client'):
    module = types.ModuleType(name)
    sys.modules[name] = modules[name] = module
    parent, _, child = name.rpartition('.')
    if parent:
        setattr(modules[parent], child, module)

class SDKError(Exception):
    code = 'MissingParameter'
    message = 'Missing VpcConfig: test-secret-value'
    requestId = 'request-id'

modules['tencentcloud.common.exception.tencent_cloud_sdk_exception'].TencentCloudSDKException = SDKError
modules['tencentcloud.common.credential'].Credential = lambda *args: None
for name in ('GetFunctionRequest', 'UpdateFunctionConfigurationRequest', 'VpcConfig', 'IntranetConfigIn'):
    setattr(modules['tencentcloud.scf.v20180416.models'], name, types.SimpleNamespace)

updates = []
polls = []
sleeps = []
states = scenario.get('states', [{'status': 'Active', 'fixed': 'DISABLE'}, {'status': 'Active', 'fixed': 'ENABLE'}])
class Client:
    def GetFunction(self, request):
        polls.append(vars(request))
        state = states[min(len(polls)-1, len(states)-1)]
        vpc = scenario.get('vpc', {'VpcId': 'vpc-existing', 'SubnetId': 'subnet-existing'})
        return types.SimpleNamespace(Status=state['status'],
            VpcConfig=types.SimpleNamespace(**vpc) if vpc else None,
            IntranetConfig=types.SimpleNamespace(IpFixed=state['fixed']))

    def UpdateFunctionConfiguration(self, request):
        updates.append(json.loads(json.dumps(vars(request), default=vars)))

modules['tencentcloud.scf.v20180416.scf_client'].ScfClient = lambda *args: Client()
os.environ['TENCENT_SECRET_ID'] = 'test-secret-id'
os.environ['TENCENT_SECRET_KEY'] = 'test-secret-value'
os.environ['SCF_REGION'] = 'ap-guangzhou'
script = runpy.run_path('tools/scf-network.py')
script['time'].sleep = lambda seconds: sleeps.append(seconds)
failure = None
with contextlib.redirect_stdout(io.StringIO()):
    try:
        script['configure']('authcenter')
    except RuntimeError as error:
        failure = str(error)
print(json.dumps({'updates': updates, 'polls': polls, 'sleeps': sleeps, 'failure': failure,
    'diagnostic': script['failure_message'](SDKError())}))
`

function configure(scenario: Record<string, unknown> = {}) {
  return JSON.parse(
    execFileSync('python3', ['-B', '-c', harness, JSON.stringify(scenario)], {
      encoding: 'utf8',
      env: { ...process.env, TMPDIR: resolve('.tmp'), PYTHONDONTWRITEBYTECODE: '1' },
    }),
  ) as {
    updates: Record<string, unknown>[]
    polls: Record<string, unknown>[]
    sleeps: number[]
    failure: string | null
    diagnostic: string
  }
}

describe('SCF fixed intranet configuration', () => {
  it('resends the deployed VPC and subnet when enabling a fixed intranet IP', () => {
    const result = configure()
    expect(result.failure).toBeNull()
    expect(result.updates).toEqual([
      {
        FunctionName: 'authcenter',
        Namespace: 'default',
        VpcConfig: { VpcId: 'vpc-existing', SubnetId: 'subnet-existing' },
        IntranetConfig: { IpFixed: 'ENABLE' },
      },
    ])
    expect(result.polls).toHaveLength(2)
  })
  it('does not update a function that already has a fixed IP', () => {
    const result = configure({ states: [{ status: 'Active', fixed: 'ENABLE' }] })
    expect(result.updates).toEqual([])
    expect(result.failure).toBeNull()
  })
  it.each([null, { VpcId: '', SubnetId: 'subnet' }, { VpcId: 'vpc', SubnetId: '' }])(
    'rejects incomplete VPC configuration before sending an update: %s',
    (vpc) => {
      const result = configure({ vpc })
      expect(result.failure).toContain('existing VPC and subnet')
      expect(result.updates).toEqual([])
    },
  )
  it('waits for both active status and the fixed IP setting', () => {
    const result = configure({
      states: [
        { status: 'Updating', fixed: 'DISABLE' },
        { status: 'Active', fixed: 'DISABLE' },
        { status: 'Updating', fixed: 'DISABLE' },
        { status: 'Active', fixed: 'DISABLE' },
        { status: 'Active', fixed: 'ENABLE' },
      ],
    })
    expect(result.failure).toBeNull()
    expect(result.polls).toHaveLength(5)
    expect(result.sleeps).toEqual([3, 3, 3])
  })
  it('reports terminal update failure', () => {
    const result = configure({
      states: [
        { status: 'Active', fixed: 'DISABLE' },
        { status: 'UpdateFailed', fixed: 'DISABLE' },
      ],
    })
    expect(result.failure).toBe('SCF function configuration failed')
  })
  it('includes the missing parameter and request ID without disclosing credentials', () => {
    const result = configure()
    expect(result.diagnostic).toBe(
      'MissingParameter: Missing VpcConfig: [redacted] (RequestId: request-id)',
    )
    expect(result.diagnostic).not.toContain('test-secret-value')
  })
})
