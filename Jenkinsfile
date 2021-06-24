// Uses Declarative syntax to run commands inside a container.
pipeline {
    triggers {
        pollSCM("*/5 * * * *")
    }
    agent {
        kubernetes {
            yaml '''
apiVersion: v1
kind: Pod
spec:
  volumes:
    - name: docker-sock
      hostPath:
        path: /var/run/docker.sock
  containers:
  - name: docker
    imagePullPolicy: Always
    image: quay.imanuel.dev/dockerhub/library---docker:stable
    command:
    - cat
    tty: true
    volumeMounts:
    - mountPath: /var/run/docker.sock
      name: docker-sock
'''
            defaultContainer 'docker'
        }
    }
    stages {
        stage('Push') {
            steps {
                container('docker') {
                    sh "docker build -t quay.imanuel.dev/imanuel/quay-mirror-version-update:$BUILD_NUMBER -f ./Dockerfile ."
                    sh "docker tag quay.imanuel.dev/imanuel/quay-mirror-version-update:$BUILD_NUMBER quay.imanuel.dev/imanuel/quay-mirror-version-update:latest"
                    sh "docker tag quay.imanuel.dev/imanuel/quay-mirror-version-update:$BUILD_NUMBER iulbricht/quay-mirror-version-update:$BUILD_NUMBER"
                    sh "docker tag quay.imanuel.dev/imanuel/quay-mirror-version-update:$BUILD_NUMBER iulbricht/quay-mirror-version-update:latest"

                    withDockerRegistry(credentialsId: 'quay.imanuel.dev', url: 'https://quay.imanuel.dev') {
                        sh "docker push quay.imanuel.dev/imanuel/quay-mirror-version-update:$BUILD_NUMBER"
                        sh "docker push quay.imanuel.dev/imanuel/quay-mirror-version-update:latest"
                    }

                    withDockerRegistry(credentialsId: 'hub.docker.com', url: '') {
                        sh "docker push iulbricht/quay-mirror-version-update:$BUILD_NUMBER"
                        sh "docker push iulbricht/quay-mirror-version-update:latest"
                    }
                }
            }
        }
    }
}
