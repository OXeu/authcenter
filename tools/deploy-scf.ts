import { spawn } from 'node:child_process'
import {
  chmodSync,
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { resolve } from 'node:path'
import { parseEnv } from 'node:util'

const root = process.cwd()
const scratch = resolve(root, '.tmp')
const packagePath = resolve(scratch, 'scf')
const projectPath = resolve(scratch, 'scf-project')
const logPath = resolve(scratch, 'scf-deploy.log')
const local = existsSync('.env') ? parseEnv(readFileSync('.env', 'utf8')) : {}
const environment: NodeJS.ProcessEnv = {
  ...local,
  ...process.env,
  TMPDIR: scratch,
  SERVERLESS_PLATFORM_VENDOR: 'tencent',
}
const runtime: NodeJS.ProcessEnv = environment.SCF_RUNTIME_ENV
  ? parseEnv(environment.SCF_RUNTIME_ENV)
  : environment
const bootstrapKeys = [
  'AC_CONFIGCENTER_URL',
  'AC_CONFIGCENTER_API_KEY',
  'AC_CONFIGCENTER_ENVIRONMENT',
]
const deploymentKeys = [
  'TENCENT_SECRET_ID',
  'TENCENT_SECRET_KEY',
  'SCF_REGION',
  'SCF_VPC_ID',
  'SCF_SUBNET_ID',
]

function run(command: string, args: string[], cwd = root): Promise<void> {
  const descriptor = openSync(logPath, 'a', 0o600)
  return new Promise((resolveRun, reject) => {
    const child = spawn(command, args, {
      cwd,
      env: environment,
      stdio: ['ignore', descriptor, descriptor],
    })
    closeSync(descriptor)
    child.once('error', () => reject(new Error(`Could not start ${command}`)))
    child.once('close', (code) =>
      code === 0
        ? resolveRun()
        : reject(new Error(`${command} failed; inspect the restricted .tmp/scf-deploy.log`)),
    )
  })
}

async function deploy() {
  for (const key of deploymentKeys)
    if (!environment[key]) throw new Error(`Missing deployment setting: ${key}`)
  for (const key of bootstrapKeys)
    if (!runtime[key]) throw new Error(`Missing runtime bootstrap setting: ${key}`)
  const configURL = new URL(runtime.AC_CONFIGCENTER_URL!)
  if (configURL.protocol !== 'https:')
    throw new Error('AC_CONFIGCENTER_URL must use HTTPS for SCF deployment')
  mkdirSync(scratch, { recursive: true })
  writeFileSync(logPath, '', { mode: 0o600 })
  chmodSync(logPath, 0o600)
  try {
    process.stdout.write('Building the Linux SCF package.\n')
    await run('make', ['scf-package'])
    mkdirSync(projectPath, { recursive: true })
    const template = readFileSync('serverless.yml', 'utf8')
    if (!template.includes('src: ./.tmp/scf')) throw new Error('Unexpected SCF package path')
    writeFileSync(
      resolve(projectPath, 'serverless.yml'),
      template.replace('src: ./.tmp/scf', 'src: ../scf'),
    )
    writeFileSync(
      resolve(packagePath, '.env.local'),
      bootstrapKeys.map((key) => `${key}=${JSON.stringify(runtime[key])}`).join('\n') + '\n',
      { mode: 0o600 },
    )
    process.stdout.write('Installing the pinned SCF CLI.\n')
    await run('npm', [
      'install',
      '--prefix',
      '.tmp/scf-cli',
      '--no-audit',
      '--no-fund',
      'serverless-cloud-framework@1.4.1',
    ])
    process.stdout.write('Deploying authcenter to Tencent SCF.\n')
    await run(
      resolve(scratch, 'scf-cli/node_modules/.bin/scf'),
      ['deploy', '--stage', 'production'],
      projectPath,
    )
    process.stdout.write('SCF deployment completed.\n')
    rmSync(logPath)
  } finally {
    rmSync(packagePath, { recursive: true, force: true })
    rmSync(projectPath, { recursive: true, force: true })
  }
}

try {
  await deploy()
} catch (error) {
  process.stderr.write((error instanceof Error ? error.message : 'SCF deployment failed') + '\n')
  process.exitCode = 1
}
